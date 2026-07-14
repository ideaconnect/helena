package scripting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/idct/helena/internal/model"
)

// bindHelena attaches the helena.* surface (env.get / env.set and the
// helena.vars.get alias). The same surface is bound in pre- and
// post-response phases so user scripts can rely on identical APIs in
// both. The session overlay is the only place script writes land.
func (rt *Runtime) bindHelena(ctx context.Context, cfg runConfig, vm *goja.Runtime) error {
	helena := vm.NewObject()

	env := vm.NewObject()
	if err := env.Set("get", func(call goja.FunctionCall) goja.Value {
		v, _ := rt.env.Get(call.Argument(0).String())
		return vm.ToValue(v)
	}); err != nil {
		return err
	}
	if err := env.Set("set", func(call goja.FunctionCall) goja.Value {
		rt.env.Set(call.Argument(0).String(), call.Argument(1).String())
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := helena.Set("env", env); err != nil {
		return err
	}

	varsObj := vm.NewObject()
	if err := varsObj.Set("get", func(call goja.FunctionCall) goja.Value {
		v, _ := rt.env.Get(call.Argument(0).String())
		return vm.ToValue(v)
	}); err != nil {
		return err
	}
	if err := helena.Set("vars", varsObj); err != nil {
		return err
	}

	// helena.interpolate(template) resolves {{var}} references the same way the
	// host resolves a request's URL / headers / body (#92). The resolver is
	// injected per-call via WithInterpolator; with none supplied it is identity.
	if err := helena.Set("interpolate", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		if cfg.interpolate != nil {
			s = cfg.interpolate(s)
		}
		return vm.ToValue(s)
	}); err != nil {
		return err
	}

	// helena.sendRequest(spec) performs an ad-hoc HTTP request through the host
	// and returns a response object identical in shape to the post-response
	// `response` global (#92). It is injected per-call via WithRequester; with
	// none wired (e.g. unit tests with no client) it throws.
	if err := helena.Set("sendRequest", func(call goja.FunctionCall) goja.Value {
		if cfg.requester == nil {
			panic(vm.NewTypeError("helena.sendRequest is unavailable in this context"))
		}
		spec, err := parseSendSpec(vm, call.Argument(0))
		if err != nil {
			panic(vm.NewTypeError("helena.sendRequest: " + err.Error()))
		}
		resp, err := cfg.requester(spec)
		if err != nil {
			panic(vm.ToValue(vm.NewGoError(err)))
		}
		return responseToObject(vm, resp)
	}); err != nil {
		return err
	}

	// helena.cookies reads the host cookie jar (#92): get(url, name) -> value or
	// undefined; getAll(url) -> { name: value }. Injected via WithCookies; with
	// none wired both return empty (reading cookies is a no-op, never an error).
	lookupCookies := func(rawURL string) []Cookie {
		if cfg.cookies == nil {
			return nil
		}
		return cfg.cookies(rawURL)
	}
	cookiesObj := vm.NewObject()
	_ = cookiesObj.Set("get", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(1).String()
		for _, c := range lookupCookies(call.Argument(0).String()) {
			if c.Name == name {
				return vm.ToValue(c.Value)
			}
		}
		return goja.Undefined()
	})
	_ = cookiesObj.Set("getAll", func(call goja.FunctionCall) goja.Value {
		obj := vm.NewObject()
		for _, c := range lookupCookies(call.Argument(0).String()) {
			_ = obj.Set(c.Name, c.Value)
		}
		return obj
	})
	if err := helena.Set("cookies", cookiesObj); err != nil {
		return err
	}

	// helena.runner steers a headless run (#92): stop() halts after this request,
	// skip() skips this request's send (pre-request only). No-ops when no runner
	// is wired (a UI Send).
	runnerObj := vm.NewObject()
	_ = runnerObj.Set("stop", func(goja.FunctionCall) goja.Value {
		if cfg.runner != nil {
			cfg.runner.Stop()
		}
		return goja.Undefined()
	})
	_ = runnerObj.Set("skip", func(goja.FunctionCall) goja.Value {
		if cfg.runner != nil {
			cfg.runner.Skip()
		}
		return goja.Undefined()
	})
	if err := helena.Set("runner", runnerObj); err != nil {
		return err
	}

	if err := rt.bindHelpers(ctx, vm, helena); err != nil {
		return err
	}

	return vm.Set("helena", helena)
}

// bindConsole installs console.log / info / error / warn. Each call
// appends one line — space-joined arguments — to res.Console so the UI
// can show the user what their script printed during the last run.
func bindConsole(vm *goja.Runtime, res *Result) {
	emit := func(prefix string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, a := range call.Arguments {
				parts = append(parts, stringify(a))
			}
			line := strings.Join(parts, " ")
			if prefix != "" {
				line = prefix + line
			}
			res.Console = append(res.Console, line)
			return goja.Undefined()
		}
	}
	console := vm.NewObject()
	_ = console.Set("log", emit(""))
	_ = console.Set("info", emit(""))
	_ = console.Set("error", emit("ERROR: "))
	_ = console.Set("warn", emit("WARN: "))
	_ = vm.Set("console", console)
}

// parseSendSpec reads a helena.sendRequest argument object into a SendSpec.
// Only `url` is required; `method` defaults later to GET, `headers` is an
// optional name→value object, and `body` is an optional string.
func parseSendSpec(vm *goja.Runtime, arg goja.Value) (SendSpec, error) {
	obj, ok := arg.(*goja.Object)
	if !ok || obj == nil {
		return SendSpec{}, errors.New("argument must be an object like {url, method, headers, body}")
	}
	spec := SendSpec{Headers: map[string]string{}}
	if s, ok := safeString(obj.Get("url")); ok {
		spec.URL = s
	}
	if spec.URL == "" {
		return SendSpec{}, errors.New("url is required")
	}
	if s, ok := safeString(obj.Get("method")); ok {
		spec.Method = strings.ToUpper(s)
	}
	if s, ok := safeString(obj.Get("body")); ok {
		spec.Body = s
	}
	if h := obj.Get("headers"); h != nil {
		if ho, ok := h.(*goja.Object); ok && ho != nil {
			for _, k := range ho.Keys() {
				if v, ok := safeString(ho.Get(k)); ok {
					spec.Headers[k] = v
				}
			}
		}
	}
	return spec, nil
}

// stringify renders a goja.Value the way `console.log` would: strings
// pass through, null/undefined become their names, everything else goes
// through json.Marshal so objects/arrays show useful structure rather
// than `[object Object]`.
func stringify(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	exported := v.Export()
	if s, ok := exported.(string); ok {
		return s
	}
	b, err := json.Marshal(exported)
	if err != nil {
		// Unmarshalable host values (functions, channels, certain host objects):
		// fall back to the value's JS string form, never Go's %v (which leaks
		// pointer addresses and is non-deterministic across runs).
		return v.String()
	}
	return string(b)
}

// interruptGrace is how long runWithTimeout waits for the VM to unwind to an
// interrupt checkpoint after signalling a timeout/cancel. goja's Interrupt only
// fires at interpreter checkpoints, so a script stuck inside a native built-in
// (a Go binding, a huge JSON.parse, etc.) never observes it — past this grace
// we abandon the goroutine and return rather than freezing the caller.
var interruptGrace = 250 * time.Millisecond

// runWithTimeout executes src on vm with a hard ScriptTimeout cap, also
// interrupting if ctx is cancelled. RunString runs on its own goroutine: on a
// normal finish we return its result; on timeout/cancel we signal Interrupt and
// wait only interruptGrace before returning regardless, so a runaway script
// that ignores Interrupt (stuck in native code) can't hold the Send worker —
// the orphaned goroutine drains into the buffered channel and exits on its own.
// Each Run* uses a fresh goja.Runtime, so an abandoned VM never affects a later
// run. Interruptions surface as the captured reason ("script execution timed
// out" / "… cancelled: …") rather than the raw goja.InterruptedError.
func runWithTimeout(ctx context.Context, vm *goja.Runtime, src string) error {
	// Compile (or fetch the cached program) before spawning the worker: a
	// syntax error surfaces synchronously with the same source positions
	// vm.RunString reported (minus its doubled "SyntaxError:" prefix), and a
	// repeated script skips recompilation entirely.
	prog, err := compileCached(src)
	if err != nil {
		return err
	}
	return runGuarded(ctx, vm, func() error {
		_, err := vm.RunProgram(prog)
		return err
	})
}

// runGuarded runs work — the ONLY thing permitted to drive vm — on its own
// goroutine under a hard ScriptTimeout cap that also honours ctx cancellation.
// On timeout/cancel it signals goja's Interrupt and waits only interruptGrace
// before returning regardless, so work that ignores Interrupt (stuck in native
// code) can't hold the caller — the orphaned goroutine drains into the buffered
// channel and exits on its own. Each caller uses a fresh goja.Runtime, so an
// abandoned VM never affects a later run.
//
// This guards not just script execution but the post-run read-back of the
// request object: its property getters are attacker-controllable JS (a script
// can install `Object.defineProperty(request,'method',{get(){while(1){}}})`),
// so the read-back must be interruptible too — otherwise it wedges the Send
// worker forever after the script's own guard has already been torn down.
func runGuarded(ctx context.Context, vm *goja.Runtime, work func() error) error {
	resultCh := make(chan error, 1) // buffered: a late finish never blocks the goroutine
	var (
		mu     sync.Mutex
		reason string
	)
	setReason := func(s string) {
		mu.Lock()
		if reason == "" {
			reason = s
		}
		mu.Unlock()
		vm.Interrupt(s)
	}
	finalize := func(err error) error {
		if err == nil {
			return nil
		}
		var ie *goja.InterruptedError
		if errors.As(err, &ie) {
			mu.Lock()
			r := reason
			mu.Unlock()
			if r == "" {
				r = fmt.Sprintf("script interrupted: %v", ie.Value())
			}
			return errors.New(r)
		}
		return err
	}

	timer := time.NewTimer(ScriptTimeout)
	defer timer.Stop()
	go func() { resultCh <- work() }()

	select {
	case err := <-resultCh:
		return finalize(err)
	case <-timer.C:
		setReason("script execution timed out")
	case <-ctx.Done():
		setReason("script execution cancelled: " + ctx.Err().Error())
	}

	// Interrupt signalled; give the VM a brief grace to unwind, then abandon.
	select {
	case err := <-resultCh:
		return finalize(err)
	case <-time.After(interruptGrace):
		mu.Lock()
		r := reason
		mu.Unlock()
		if r == "" {
			r = "script execution timed out"
		}
		return errors.New(r)
	}
}

// requestToObject mirrors the model.Request into a goja object the
// script can mutate. Headers, params, and form fields expose only
// enabled rows as flat name → value objects so scripts can naturally do
// `request.headers["X-Trace"] = "abc"` or `delete request.params.token`.
// `request.form` is bound only for form-urlencoded / multipart bodies
// where the model has structured fields; otherwise the property is left
// unset so scripts can probe its existence as a body-type hint.
func requestToObject(vm *goja.Runtime, r *model.Request) *goja.Object {
	obj := vm.NewObject()
	method := string(r.Method)
	if method == "" {
		method = string(model.GET)
	}
	_ = obj.Set("method", method)
	_ = obj.Set("url", r.URL)
	_ = obj.Set("body", r.Body.Content)
	_ = obj.Set("bodyType", string(r.Body.Type))
	_ = obj.Set("headers", kvToObject(vm, r.Headers))
	_ = obj.Set("params", kvToObject(vm, r.Params))
	if r.Body.Type == model.BodyForm || r.Body.Type == model.BodyMultipart {
		_ = obj.Set("form", kvToObject(vm, r.Body.Form))
	}
	return obj
}

// kvToObject builds a plain JS object from a KeyValue slice, skipping
// disabled rows. Duplicate keys land last-write-wins, matching how most
// scripting authors think of header tables.
func kvToObject(vm *goja.Runtime, kvs []model.KeyValue) *goja.Object {
	obj := vm.NewObject()
	for _, kv := range kvs {
		if !kv.Enabled {
			continue
		}
		_ = obj.Set(kv.Key, kv.Value)
	}
	return obj
}

// writeBackGuarded runs writeBackRequest under runGuarded so a hostile accessor
// on the request object — e.g. a getter installed via Object.defineProperty, or
// a value with an infinite-loop toString — can be interrupted instead of
// wedging the Send worker forever. Without this, the read-back runs after the
// script's own timeout guard is already gone, with no interrupt armed.
//
// The read-back mutates a COPY; r is updated only on success. So if the
// read-back is abandoned (stuck in native code past the grace), the orphaned
// goroutine keeps writing the throwaway copy and never races the caller's
// request. writeBackRequest reassigns whole slices (via mergeKVFromObject)
// rather than mutating in place, so the copy shares no mutable backing with r.
func writeBackGuarded(ctx context.Context, vm *goja.Runtime, obj *goja.Object, r *model.Request) error {
	tmp := *r
	if err := runGuarded(ctx, vm, func() error { return writeBackRequest(obj, &tmp) }); err != nil {
		return err
	}
	*r = tmp
	return nil
}

// writeBackRequest reads the (possibly mutated) JS request object back
// into r. Disabled rows in the original headers/params slices pass
// through untouched so an inherited disabled header still survives a
// script run; enabled rows are merged with the JS-side object via
// mergeKVFromObject. Every value read goes through safeString so a
// hostile toString that throws turns into an empty string rather than a
// process-crashing panic.
func writeBackRequest(obj *goja.Object, r *model.Request) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("script mutation panicked while reading back request: %v", rec)
		}
	}()
	if s, ok := safeString(obj.Get("method")); ok {
		r.Method = model.Method(s)
	}
	if s, ok := safeString(obj.Get("url")); ok {
		r.URL = s
	}
	if s, ok := safeString(obj.Get("body")); ok {
		r.Body.Content = s
	}
	if v := obj.Get("headers"); v != nil {
		if o, ok := v.(*goja.Object); ok && o != nil {
			r.Headers = mergeKVFromObject(r.Headers, o)
		}
	}
	if v := obj.Get("params"); v != nil {
		if o, ok := v.(*goja.Object); ok && o != nil {
			r.Params = mergeKVFromObject(r.Params, o)
		}
	}
	if v := obj.Get("form"); v != nil {
		if o, ok := v.(*goja.Object); ok && o != nil {
			r.Body.Form = mergeKVFromObject(r.Body.Form, o)
		}
	}
	return nil
}

// safeString returns v.String() or ("", false) for nil / undefined /
// null values, AND it absorbs the panic goja.Value.String triggers when
// the underlying JS value's toString throws. Returning ("", false) on
// throw is the conservative choice: the model field is left unchanged
// rather than getting a misleading "[object Object]" or similar.
func safeString(v goja.Value) (s string, ok bool) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", false
	}
	defer func() {
		if r := recover(); r != nil {
			s, ok = "", false
		}
	}()
	return v.String(), true
}

// mergeKVFromObject reconciles a KeyValue slice with the post-script JS
// object using these rules so user intent is preserved across types of
// change:
//
//   - Disabled rows in existing are passed through unchanged (the script
//     never saw them and shouldn't be able to enable them inadvertently).
//   - Enabled rows whose key is still present in obj have their value
//     updated to the JS value (case-insensitive name match, so HTTP
//     header semantics still hold).
//   - Enabled rows whose key has disappeared from obj are dropped — the
//     script called `delete request.headers["X"]` and meant it.
//   - Keys present in obj but not matched to any existing row are
//     appended as Enabled new entries.
//
// JS object keys differing only in case (`Content-Type` vs `content-type`)
// are coalesced last-write-wins before the merge so duplicates can't
// reach the model. JS preserves insertion order on string keys, so
// "later in the object" == "later in obj.Keys()" — this matches the
// natural JS dict semantics.
func mergeKVFromObject(existing []model.KeyValue, obj *goja.Object) []model.KeyValue {
	type entry struct {
		key string // original casing kept for new appends
		val string
	}
	dedup := make(map[string]entry, len(obj.Keys()))
	for _, k := range obj.Keys() {
		v, ok := safeString(obj.Get(k))
		if !ok {
			continue
		}
		dedup[strings.ToLower(k)] = entry{key: k, val: v}
	}

	consumed := make(map[string]bool, len(dedup))
	out := make([]model.KeyValue, 0, len(existing)+len(dedup))
	for _, kv := range existing {
		if !kv.Enabled {
			out = append(out, kv)
			continue
		}
		lk := strings.ToLower(kv.Key)
		e, ok := dedup[lk]
		if !ok {
			continue // script deleted it
		}
		kv.Value = e.val
		consumed[lk] = true
		out = append(out, kv)
	}
	// Append script-added entries in the JS object's insertion order (preserved
	// by obj.Keys()), NOT Go map-iteration order — otherwise the on-the-wire
	// order of script-added fields is non-deterministic run to run, which breaks
	// signed requests and golden-output tests.
	for _, k := range obj.Keys() {
		lk := strings.ToLower(k)
		e, ok := dedup[lk]
		if !ok || consumed[lk] {
			continue
		}
		consumed[lk] = true // dedup duplicate-casing keys to one append
		out = append(out, model.KeyValue{Enabled: true, Key: e.key, Value: e.val})
	}
	return out
}

// chainToObject builds the script-side `chain` global from the chain
// map supplied by [internal/chain]. Each alias becomes a property
// whose value is `{ request: {...}, response: {...} }` mirroring the
// top-level request/response surface. JSON / XML parsing on each
// chain entry's response is **lazy** — accessed via JS getter
// properties so a leaf script that only touches
// `chain.login.response.body` doesn't pay the full json.Unmarshal /
// XML walk cost for every chained predecessor (which can dominate
// Send time when chains have many large-bodied steps).
//
// A nil/empty map binds an empty object so scripts can still safely
// `Object.keys(chain)` without a type check.
func chainToObject(vm *goja.Runtime, chain map[string]ChainView) *goja.Object {
	obj := vm.NewObject()
	for alias, view := range chain {
		entry := vm.NewObject()
		req := vm.NewObject()
		_ = req.Set("method", view.Request.Method)
		_ = req.Set("url", view.Request.URL)
		_ = req.Set("body", string(view.Request.Body))
		_ = entry.Set("request", req)
		_ = entry.Set("response", lazyResponseToObject(vm, view.Response))
		_ = obj.Set(alias, entry)
	}
	return obj
}

// lazyResponseToObject is responseToObject with JSON / XML bodies
// behind goja accessor properties: the body bytes are decoded only on
// the first read, then cached on a closure so subsequent reads are
// constant-time. Used for chain entries where most callers ignore the
// parsed bodies entirely — the top-level response global stays eager
// (responseToObject) because the user opted into it by writing a
// post-response script.
func lazyResponseToObject(vm *goja.Runtime, in ResponseInput) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("status", in.StatusCode)
	_ = obj.Set("statusText", in.Status)
	body := string(in.Body)
	_ = obj.Set("body", body)
	_ = obj.Set("text", body)

	headers := vm.NewObject()
	for k, vs := range in.Headers {
		if len(vs) == 0 {
			continue
		}
		_ = headers.Set(textproto.CanonicalMIMEHeaderKey(k), vs[0])
	}
	_ = obj.Set("headers", headers)

	// JSON getter — parses on first access, caches the result.
	var jsonCache goja.Value
	jsonGetter := func(goja.FunctionCall) goja.Value {
		if jsonCache != nil {
			return jsonCache
		}
		if parsed, ok := tryParseJSON(in.Body); ok {
			jsonCache = vm.ToValue(parsed)
		} else {
			jsonCache = goja.Undefined()
		}
		return jsonCache
	}
	_ = obj.DefineAccessorProperty("json", vm.ToValue(jsonGetter), nil,
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// XML getter — same pattern.
	var xmlCache goja.Value
	xmlGetter := func(goja.FunctionCall) goja.Value {
		if xmlCache != nil {
			return xmlCache
		}
		if parsed, ok := tryParseXML(in.Body); ok {
			xmlCache = vm.ToValue(parsed)
		} else {
			xmlCache = goja.Undefined()
		}
		return xmlCache
	}
	_ = obj.DefineAccessorProperty("xml", vm.ToValue(xmlGetter), nil,
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	return obj
}

// responseToObject reflects the response into a goja object exposed to
// post-response scripts. JSON parsing is attempted regardless of
// Content-Type so APIs that return JSON without a header still get
// `response.json`; same for XML and `response.xml`. Both stay undefined
// for non-matching bodies.
func responseToObject(vm *goja.Runtime, in ResponseInput) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("status", in.StatusCode)
	_ = obj.Set("statusText", in.Status)
	body := string(in.Body)
	_ = obj.Set("body", body)
	_ = obj.Set("text", body)

	// Canonicalize header keys to MIME form so case-insensitive script
	// lookups work in practice. Net/http already stores canonical keys
	// (Content-Type, Location), but ResponseInput is also constructed
	// directly in tests and conceivably from other inputs that may use
	// raw casing — normalizing here makes the surface predictable.
	// We read straight from the map (vs http.Header.Get) because Get
	// canonicalizes the lookup key and so misses non-canonical entries.
	headers := vm.NewObject()
	for k, vs := range in.Headers {
		if len(vs) == 0 {
			continue
		}
		_ = headers.Set(textproto.CanonicalMIMEHeaderKey(k), vs[0])
	}
	_ = obj.Set("headers", headers)

	if parsed, ok := tryParseJSON(in.Body); ok {
		_ = obj.Set("json", vm.ToValue(parsed))
	} else {
		_ = obj.Set("json", goja.Undefined())
	}
	if parsed, ok := tryParseXML(in.Body); ok {
		_ = obj.Set("xml", vm.ToValue(parsed))
	} else {
		_ = obj.Set("xml", goja.Undefined())
	}
	return obj
}

// tryParseJSON returns (parsed, true) when body is a JSON object or
// array — Helena's two interesting cases. Top-level scalars (numbers,
// strings, booleans) are technically valid JSON but rarely the user's
// intent, so they fall back to false and stay accessible via
// response.body.
func tryParseJSON(body []byte) (interface{}, bool) {
	// Sniff on the raw bytes — a string conversion here would copy the whole
	// (possibly multi-MB) body just to look at its first non-space byte.
	trim := bytes.TrimLeft(body, " \t\r\n")
	if len(trim) == 0 {
		return nil, false
	}
	c := trim[0]
	if c != '{' && c != '[' {
		return nil, false
	}
	var v interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	return v, true
}
