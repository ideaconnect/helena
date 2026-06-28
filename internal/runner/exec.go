package runner

import (
	"context"
	"strings"

	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/vars"
)

// headlessExecutor runs pre-script → http → post-script for one request and
// packages the response as a chain.View. It mirrors the UI's chainExecutor
// (and the integration test one) but layers the full resolver scope chain so a
// CLI run resolves variables identically to a GUI Send.
type headlessExecutor struct {
	sess       *session.Session
	client     *httpclient.Client
	rt         *scripting.Runtime
	globalSnap map[string]string
	dotEnvSnap map[string]string
	colSnap    map[string]string
	envSnap    map[string]string
}

// ExecuteOnce satisfies chain.Executor for the chain runner (chain steps don't
// surface their test results to the report; only the leaf's do via executeOnce).
func (e headlessExecutor) ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []string, error) {
	view, _, err := e.executeOnce(ctx, r, chainMap)
	return view, nil, err
}

// executeOnce is ExecuteOnce plus the recorded test()/expect() results, used
// for the leaf request so the report can show its assertions.
func (e headlessExecutor) executeOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []scripting.TestResult, error) {
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)

	scriptChain := chainViewToScripting(chainMap)

	// interp backs helena.interpolate (#92): same scope chain as the request send,
	// rebuilt per call so in-script helena.env.set writes are reflected. No prompt
	// scope — a headless run can't ask.
	interp := func(s string) string {
		rr := vars.New(e.globalSnap, e.dotEnvSnap, e.colSnap, e.envSnap, enabledVars(r.Variables), e.sess.SnapshotEnvOverlay()).
			WithFallback(vars.Compose(chain.VarLookup(chainMap), vars.Dynamic))
		out, _ := rr.Resolve(s)
		return out
	}

	preRes, preErr := e.rt.RunPreRequest(ctx, r.Scripts.PreRequest, &r, scriptChain, scripting.WithInterpolator(interp))
	if preErr != nil {
		return chain.View{}, preRes.Tests, preErr
	}

	// Same scope order as the UI Send: global < .env < collection < env <
	// request vars < script overlay. No prompt scope — a headless run can't ask.
	resolver := vars.New(e.globalSnap, e.dotEnvSnap, e.colSnap, e.envSnap, enabledVars(r.Variables), e.sess.SnapshotEnvOverlay()).
		WithFallback(vars.Compose(chain.VarLookup(chainMap), vars.Dynamic))
	resp, err := e.client.Do(ctx, r, resolver)
	if err != nil {
		return chain.View{}, preRes.Tests, err
	}

	view := chain.View{
		Request: chain.RequestView{Method: string(r.Method), URL: resp.RequestURL, Body: resp.RequestBody},
		Response: chain.ResponseView{
			StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers,
			Body: resp.Body, Size: resp.Size, Duration: resp.Duration, CORSWarning: resp.CORSWarning,
		},
	}
	postRes, postErr := e.rt.RunPostResponse(ctx, r.Scripts.PostResponse, r,
		scripting.ResponseInput{StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers, Body: resp.Body},
		scriptChain, scripting.WithInterpolator(interp))
	tests := append(append([]scripting.TestResult(nil), preRes.Tests...), postRes.Tests...)
	if postErr != nil {
		return view, tests, postErr
	}
	return view, tests, nil
}

// enabledVars flattens a request's variables into a resolver scope.
func enabledVars(vs []model.Variable) map[string]string {
	m := make(map[string]string, len(vs))
	for _, v := range vs {
		if v.Enabled {
			m[v.Key] = v.Value
		}
	}
	return m
}

// envBridge wires scripting.EnvBridge to the session: reads layer the captured
// snapshot over the live overlay; writes hit the overlay only.
type envBridge struct {
	s    *session.Session
	base map[string]string
}

func (b envBridge) Get(name string) (string, bool) {
	if v, ok := b.s.EnvOverlay(name); ok {
		return v, true
	}
	if v, ok := b.base[name]; ok {
		return v, true
	}
	return "", false
}

func (b envBridge) Set(name, value string) { b.s.SetEnvOverlay(name, value) }

// nilFinder mirrors the cold-start finder: every lookup misses so chain.Resolve
// still walks an empty chain without a collection-aware finder.
type nilFinder struct{}

func (nilFinder) FindRequestByPath(string) (model.Request, bool) { return model.Request{}, false }
func (nilFinder) FindRequestByID(string) (model.Request, bool)   { return model.Request{}, false }

// chainViewToScripting converts the chain runner's View map into the scripting
// binding's ChainView map (same shape, different packages by design — AGENTS
// invariant 15).
func chainViewToScripting(in map[string]chain.View) map[string]scripting.ChainView {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]scripting.ChainView, len(in))
	for alias, v := range in {
		out[alias] = scripting.ChainView{
			Request:  scripting.ChainRequestView{Method: v.Request.Method, URL: v.Request.URL, Body: v.Request.Body},
			Response: scripting.ResponseInput{StatusCode: v.Response.StatusCode, Status: v.Response.Status, Headers: v.Response.Headers, Body: v.Response.Body},
		}
	}
	return out
}

// collectRequests walks the active collection depth-first, returning every
// request node with its collection-relative human path (e.g. "Auth/Login").
func collectRequests(tree *session.Tree) []reqRef {
	var out []reqRef
	for _, root := range tree.ChildIDs("") {
		walk(tree, root, "", &out)
	}
	return out
}

func walk(tree *session.Tree, id, prefix string, out *[]reqRef) {
	if r, ok := tree.Request(id); ok {
		path := r.Name
		if prefix != "" {
			path = prefix + "/" + r.Name
		}
		*out = append(*out, reqRef{id: id, path: path, req: *r})
		return
	}
	// Folder (not the collection root) contributes its label to the path; the
	// collection root is skipped so paths are collection-relative.
	next := prefix
	if strings.Contains(id, "/") {
		if label := tree.Label(id); label != "" {
			if next != "" {
				next += "/"
			}
			next += label
		}
	}
	for _, child := range tree.ChildIDs(id) {
		walk(tree, child, next, out)
	}
}
