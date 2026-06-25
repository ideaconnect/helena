package ui

import (
	"context"
	"fmt"

	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/vars"
)

// sessionEnvBridge adapts *session.Session to scripting.EnvBridge so the
// scripting package stays decoupled from internal/session.
//
// `base` is a snapshot of the active environment captured on the UI
// goroutine at Send entry; the script reads through it without
// touching live session state, which avoids racing against UI-thread
// mutations of s.cols / s.activeCol / Environment.Variables during a
// long-running script. `s` is still consulted for overlay writes and
// reads — the overlay carries its own RWMutex so live access is safe.
//
// Get returns overlay-over-snapshot. Set writes only to the session's
// in-memory overlay (invariant 9 — script-set vars never touch disk).
type sessionEnvBridge struct {
	s    *session.Session
	base map[string]string
}

func (b sessionEnvBridge) Get(name string) (string, bool) {
	if v, ok := b.s.EnvOverlay(name); ok {
		return v, true
	}
	v, ok := b.base[name]
	return v, ok
}

func (b sessionEnvBridge) Set(name, value string) { b.s.SetEnvOverlay(name, value) }

// nilFinder is a chain.RequestFinder used when no collection is loaded —
// every lookup returns false. Lets chain.Resolve still walk the leaf's
// (empty) chain without crashing.
type nilFinder struct{}

func (nilFinder) FindRequestByPath(string) (model.Request, bool) {
	return model.Request{}, false
}

func (nilFinder) FindRequestByID(string) (model.Request, bool) {
	return model.Request{}, false
}

// chainExecutor is the single execution path for both chain steps and
// the leaf: deep-copy the request's KV slices, run the pre-script with
// the supplied chain map bound as `chain.<alias>`, build the resolver
// from the captured env snapshot + a fresh overlay snapshot, call
// client.Do, then run the post-script. The captured fields close over
// the per-Send constants (rt, client, envSnap, sess) so successive
// ExecuteOnce calls don't repeat the boilerplate.
type chainExecutor struct {
	rt      *scripting.Runtime
	client  *httpclient.Client
	colSnap map[string]string // collection-level variables (#80), below env
	envSnap map[string]string
	sess    *session.Session
}

func (e chainExecutor) ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []string, error) {
	// Deep-copy slices to insulate the parent request's data from
	// per-step writeback or chain-time mutations.
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)

	scriptChain := chainViewToScripting(chainMap)
	var console []string

	preRes, preErr := e.rt.RunPreRequest(ctx, r.Scripts.PreRequest, &r, scriptChain)
	console = append(console, preRes.Console...)
	if preErr != nil {
		return chain.View{}, console, fmt.Errorf("pre-script: %w", preErr)
	}

	// Scopes low->high: collection (#80) < env < this request's own
	// variables (#82, highest static) < script overlay. The chain fallback
	// lets this request's URL / params / headers / body / auth use
	// {{chain.<alias>.response.json.token}}-style templates, scoped to this
	// request's own chain aliases (same map the pre-script saw); Dynamic adds
	// Postman-style {{$guid}}/{{$timestamp}}/… magic variables (#85).
	resolver := vars.New(e.colSnap, e.envSnap, enabledRequestVars(r.Variables), e.sess.SnapshotEnvOverlay()).
		WithFallback(vars.Compose(chain.VarLookup(chainMap), vars.Dynamic))
	resp, err := e.client.Do(ctx, r, resolver)
	if err != nil {
		return chain.View{}, console, err
	}

	// view.Request reflects what actually went on the wire: the
	// resolved URL (with {{vars}} substituted and query params merged)
	// and the encoded body bytes (URL-encoded form for form-urlencoded,
	// multipart envelope for multipart, raw bytes otherwise). Both
	// come from httpclient via Response so scripts reading
	// chain.<alias>.request.{url,body} see the wire form, not the
	// pre-resolution template.
	view := chain.View{
		Request: chain.RequestView{Method: string(r.Method), URL: resp.RequestURL, Body: resp.RequestBody},
		Response: chain.ResponseView{
			StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers,
			Body: resp.Body, Size: resp.Size, Duration: resp.Duration, CORSWarning: resp.CORSWarning,
		},
	}

	postRes, postErr := e.rt.RunPostResponse(ctx, r.Scripts.PostResponse, r,
		scripting.ResponseInput{StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers, Body: resp.Body},
		scriptChain)
	console = append(console, postRes.Console...)
	if postErr != nil {
		return view, console, fmt.Errorf("post-script: %w", postErr)
	}
	return view, console, nil
}

// chainViewToScripting bridges chain.View to scripting.ChainView so
// the scripting package stays unaware of internal/chain. They carry
// the same data with different field names by design (chain.View has
// display fields the scripting surface deliberately doesn't expose).
func chainViewToScripting(chainMap map[string]chain.View) map[string]scripting.ChainView {
	if len(chainMap) == 0 {
		return nil
	}
	out := make(map[string]scripting.ChainView, len(chainMap))
	for alias, v := range chainMap {
		out[alias] = scripting.ChainView{
			Request: scripting.ChainRequestView{Method: v.Request.Method, URL: v.Request.URL, Body: v.Request.Body},
			Response: scripting.ResponseInput{
				StatusCode: v.Response.StatusCode, Status: v.Response.Status,
				Headers: v.Response.Headers, Body: v.Response.Body,
			},
		}
	}
	return out
}
