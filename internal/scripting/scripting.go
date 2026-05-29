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
const ScriptTimeout = 5 * time.Second

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

// Result captures script-emitted console output. Errors thrown inside
// the script are returned via err from Run*, but any console lines
// emitted before the throw are still in Result so the UI can show the
// user how far the script got.
type Result struct {
	Console []string
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

// RunPreRequest evaluates script with the helena.* surface bound and the
// request bound as a mutable global. Mutations on request.method, url,
// body, headers, and params are merged back into r before returning.
// An empty (or whitespace-only) script returns a zero Result with no
// error.
func (rt *Runtime) RunPreRequest(ctx context.Context, script string, r *model.Request) (Result, error) {
	if strings.TrimSpace(script) == "" {
		return Result{}, nil
	}
	vm := goja.New()
	res := &Result{}
	if err := rt.bindHelena(vm); err != nil {
		return *res, err
	}
	bindConsole(vm, res)
	reqObj := requestToObject(vm, r)
	if err := vm.Set("request", reqObj); err != nil {
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
// `request` and `response` globals. Mutations on the request object are
// ignored: the request has already gone over the wire. An empty (or
// whitespace-only) script returns a zero Result with no error.
func (rt *Runtime) RunPostResponse(ctx context.Context, script string, r model.Request, in ResponseInput) (Result, error) {
	if strings.TrimSpace(script) == "" {
		return Result{}, nil
	}
	vm := goja.New()
	res := &Result{}
	if err := rt.bindHelena(vm); err != nil {
		return *res, err
	}
	bindConsole(vm, res)
	if err := vm.Set("request", requestToObject(vm, &r)); err != nil {
		return *res, err
	}
	if err := vm.Set("response", responseToObject(vm, in)); err != nil {
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
