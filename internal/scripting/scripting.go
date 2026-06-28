// Package scripting executes per-request JavaScript hooks bundled with
// Helena requests. Each Run* invocation constructs a fresh
// goja.Runtime — state never leaks between requests — binds the helena.*
// surface plus the request (mutable in the pre-request case, read-only in
// the post-response case) and the response (post only), then evaluates
// the user's source.
//
// The runtime stays decoupled from internal/session and
// internal/httpclient behind the EnvBridge interface, so scripting can be
// unit-tested with a fake bridge and the env overlay can never be
// persisted through a backdoor.
package scripting

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/idct/helena/internal/model"
)

// ScriptTimeout is the maximum wall-clock a single script gets before
// the runtime interrupts execution. Scripts exceeding it return a
// "script execution timed out" error — the in-flight request still
// completes for the pre-request case, since the interrupt fires only
// inside RunWithContext.
var ScriptTimeout = 5 * time.Second

// EnvBridge mediates a script's helena.env.* calls. Get returns the
// currently resolved value (overlay over underlying environment); Set
// writes to the session's in-memory overlay only — implementations MUST
// NOT persist to disk. The bridge is also exposed under helena.vars.get
// as a more-conventional alias.
type EnvBridge interface {
	Get(name string) (string, bool)
	Set(name, value string)
}

// Runtime executes the per-request hooks. It is safe to reuse a single
// Runtime across many requests because each Run* call constructs its
// own goja.Runtime; the only shared state is the EnvBridge.
type Runtime struct {
	env EnvBridge
}

// New builds a Runtime that shares env across all Run* invocations.
// Passing a nil bridge is valid: helena.env.set becomes a no-op and
// helena.env.get always returns "".
func New(env EnvBridge) *Runtime {
	if env == nil {
		env = nopBridge{}
	}
	return &Runtime{env: env}
}

// RunOption configures a single Run* invocation. Options are additive, so
// existing callers that pass none keep their behaviour.
type RunOption func(*runConfig)

// runConfig holds the per-call settings assembled from the RunOptions.
type runConfig struct {
	interpolate func(string) string
	requester   func(SendSpec) (ResponseInput, error)
	cookies     func(rawURL string) []Cookie
}

// Cookie is one name/value pair the host's jar would send to a URL, exposed to
// scripts via helena.cookies (#92).
type Cookie struct {
	Name  string
	Value string
}

// SendSpec is an ad-hoc HTTP request a script asks the host to perform via
// helena.sendRequest (#92). Method defaults to GET when empty; URL and header
// values are {{var}}-resolved by the host before sending, exactly like a normal
// request. Body is sent verbatim (text); set a Content-Type header for JSON etc.
type SendSpec struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// ToRequest converts the spec into a model.Request the host can send. Method
// defaults to GET; a non-empty Body becomes a text body (set a Content-Type
// header for other media types). Headers are emitted in sorted key order so the
// produced request is deterministic.
func (s SendSpec) ToRequest() model.Request {
	method := model.Method(strings.ToUpper(s.Method))
	if method == "" {
		method = model.GET
	}
	keys := make([]string, 0, len(s.Headers))
	for k := range s.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	headers := make([]model.KeyValue, 0, len(keys))
	for _, k := range keys {
		headers = append(headers, model.KeyValue{Enabled: true, Key: k, Value: s.Headers[k]})
	}
	req := model.Request{Method: method, URL: s.URL, Headers: headers}
	if s.Body != "" {
		req.Body = model.Body{Type: model.BodyText, Content: s.Body}
	}
	return req
}

// WithInterpolator supplies the function backing helena.interpolate(template)
// (#92): the host resolves `{{var}}` references in a string exactly as it does
// for a request's URL / headers / body. Callers typically pass a closure over
// the request's variable resolver (rebuilt with a fresh env-overlay snapshot per
// call so in-script helena.env.set writes are visible). When no interpolator is
// supplied, helena.interpolate returns its argument unchanged.
func WithInterpolator(fn func(string) string) RunOption {
	return func(c *runConfig) { c.interpolate = fn }
}

// WithRequester supplies the function backing helena.sendRequest(spec) (#92):
// the host performs the ad-hoc request (resolving {{vars}}, applying the cookie
// jar, honouring the Send context) and returns its response. When no requester
// is supplied, helena.sendRequest throws — it has no meaning outside a Send.
func WithRequester(fn func(SendSpec) (ResponseInput, error)) RunOption {
	return func(c *runConfig) { c.requester = fn }
}

// WithCookies supplies the function backing helena.cookies (#92): given a URL it
// returns the name/value cookies the host's jar would send there. When no
// function is supplied, helena.cookies.get returns undefined and getAll returns
// an empty object — reading cookies is a no-op rather than an error.
func WithCookies(fn func(rawURL string) []Cookie) RunOption {
	return func(c *runConfig) { c.cookies = fn }
}

func newRunConfig(opts []RunOption) runConfig {
	var c runConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Result captures script-emitted console output and test() assertions.
// Errors thrown inside the script are returned via err from Run*, but any
// console lines and test results recorded before the throw are still in
// Result so the UI can show the user how far the script got.
type Result struct {
	Console []string
	Tests   []TestResult // test()/expect() assertion outcomes (#87)
}

// TestResult is one test(name, fn) outcome (#87). Passed is false when an
// expect() matcher (or any throw) fired inside the test body; Error then holds
// the failure message.
type TestResult struct {
	Name   string
	Passed bool
	Error  string
}

// ResponseInput is the data scripting binds as the post-response
// `response` global. Carried as a small struct rather than
// *http.Response so the scripting package stays free of net/http
// server-side semantics and can be driven directly from tests.
type ResponseInput struct {
	StatusCode int
	Status     string // full text e.g. "200 OK"
	Headers    http.Header
	Body       []byte
}

// ChainView is one entry in the script-side `chain.<alias>` global —
// the snapshot of a previously executed chain step, mirroring the
// shape of the top-level `request` + `response` globals. The
// internal/chain package builds these from real Send results.
type ChainView struct {
	Request  ChainRequestView
	Response ResponseInput
}

// ChainRequestView is the immutable request side of a chain entry —
// what method, URL, and body the chained predecessor was sent with.
type ChainRequestView struct {
	Method string
	URL    string
	Body   []byte
}

// RunPreRequest evaluates script with the helena.* surface bound, the
// request bound as a mutable global, and any chain predecessors bound
// as `chain.<alias>` (read-only). Mutations on request.method, url,
// body, headers, params, and form are merged back into r before
// returning. An empty (or whitespace-only) script returns a zero
// Result with no error. A nil chain map binds an empty `chain` global.
func (rt *Runtime) RunPreRequest(ctx context.Context, script string, r *model.Request, chain map[string]ChainView, opts ...RunOption) (Result, error) {
	if strings.TrimSpace(script) == "" {
		return Result{}, nil
	}
	vm := goja.New()
	res := &Result{}
	if err := rt.bindHelena(ctx, newRunConfig(opts), vm); err != nil {
		return *res, err
	}
	bindConsole(vm, res)
	if err := bindTest(vm, res); err != nil {
		return *res, err
	}
	reqObj := requestToObject(vm, r)
	if err := vm.Set("request", reqObj); err != nil {
		return *res, err
	}
	if err := vm.Set("chain", chainToObject(vm, chain)); err != nil {
		return *res, err
	}
	if err := runWithTimeout(ctx, vm, script); err != nil {
		return *res, err
	}
	if err := writeBackRequest(reqObj, r); err != nil {
		return *res, err
	}
	return *res, nil
}

// RunPostResponse evaluates script with helena.* plus read-only
// `request`, `response`, and `chain.<alias>` globals. Mutations on
// the request object are ignored: the request has already gone over
// the wire. An empty (or whitespace-only) script returns a zero
// Result with no error.
func (rt *Runtime) RunPostResponse(ctx context.Context, script string, r model.Request, in ResponseInput, chain map[string]ChainView, opts ...RunOption) (Result, error) {
	if strings.TrimSpace(script) == "" {
		return Result{}, nil
	}
	vm := goja.New()
	res := &Result{}
	if err := rt.bindHelena(ctx, newRunConfig(opts), vm); err != nil {
		return *res, err
	}
	bindConsole(vm, res)
	if err := bindTest(vm, res); err != nil {
		return *res, err
	}
	if err := vm.Set("request", requestToObject(vm, &r)); err != nil {
		return *res, err
	}
	if err := vm.Set("response", responseToObject(vm, in)); err != nil {
		return *res, err
	}
	if err := vm.Set("chain", chainToObject(vm, chain)); err != nil {
		return *res, err
	}
	if err := runWithTimeout(ctx, vm, script); err != nil {
		return *res, err
	}
	return *res, nil
}

// nopBridge is the default EnvBridge used when New is called with nil.
// It lets the runtime stay non-nil-safe for tests that don't care about
// env at all.
type nopBridge struct{}

func (nopBridge) Get(string) (string, bool) { return "", false }
func (nopBridge) Set(string, string)        {}
