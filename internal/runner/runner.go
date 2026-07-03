// Package runner executes every request in a loaded collection headlessly —
// the engine behind the `helena run` CLI (#90). It reuses the same building
// blocks as a UI Send (vars resolution, auth flattening, chain, scripting,
// httpclient, assertions) so a CI run matches what the GUI would send.
//
// NOTE: the per-request execution here intentionally duplicates the UI's
// chainExecutor (and the test-only one in internal/integration). Extracting a
// single shared execution engine is tracked as follow-up work; this package is
// additive and does not touch the UI send path.
package runner

import (
	"context"
	"time"

	"github.com/idct/helena/internal/assertion"
	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
)

// Check is one assertion or test()/expect() outcome for a request.
type Check struct {
	Name   string
	Passed bool
	Error  string
}

// RequestResult is the outcome of running a single request: what was sent, the
// response status, the per-request checks, and any execution error.
type RequestResult struct {
	Path       string // human-readable path, e.g. "Auth/Login"
	Method     string
	URL        string // the resolved URL that went on the wire (when the HTTP completed)
	StatusCode int
	Duration   time.Duration
	Err        string // non-empty on a pre-script / HTTP / post-script / chain failure
	Checks     []Check
	Skipped    bool // the pre-request script called helena.runner.skip() (#92) — no send
}

// OK reports whether the request executed without error and every check passed.
// A skipped request is OK as long as nothing failed before it was skipped.
func (r RequestResult) OK() bool {
	if r.Err != "" {
		return false
	}
	for _, c := range r.Checks {
		if !c.Passed {
			return false
		}
	}
	return true
}

// Report is the result of a full run.
type Report struct {
	Results []RequestResult
}

// Failed reports whether any request errored or any check failed (the CLI's
// non-zero exit condition).
func (rp Report) Failed() bool {
	for _, r := range rp.Results {
		if !r.OK() {
			return true
		}
	}
	return false
}

// Totals returns the number of requests run, checks passed, and checks failed.
func (rp Report) Totals() (requests, checksPassed, checksFailed int) {
	requests = len(rp.Results)
	for _, r := range rp.Results {
		for _, c := range r.Checks {
			if c.Passed {
				checksPassed++
			} else {
				checksFailed++
			}
		}
	}
	return
}

// Run executes every request in sess's active collection (depth-first) and
// returns a Report. Each request is sent independently with its own chain,
// scripts, and assertions, exactly as a UI Send would. The env overlay is
// rolled back after each request so script-set values don't leak between them.
func Run(ctx context.Context, sess *session.Session) Report {
	var rep Report
	stop := &stopSignal{} // set by helena.runner.stop() (#92)
	// One client (and connection pool) for the whole run: building it per
	// request would orphan an idle keep-alive socket per request sent.
	client := httpclient.New(sess.Settings())
	client.SetCookieJar(sess.CookieJar())
	client.SetOAuth2Resolver(auth.NewClientCredentialsResolver(
		sess.TokenCache(), nil, sess.ActiveCollectionDir()))
	defer client.CloseIdleConnections()
	for _, rq := range collectRequests(sess.Tree()) {
		rep.Results = append(rep.Results, runOne(ctx, sess, client, rq, stop))
		if stop.stopped() {
			break
		}
	}
	return rep
}

// reqRef is a request node plus its human path within the collection.
type reqRef struct {
	id   string
	path string
	req  model.Request
}

func runOne(ctx context.Context, sess *session.Session, client *httpclient.Client, rq reqRef, stop *stopSignal) RequestResult {
	res := RequestResult{Path: rq.path, Method: string(rq.req.Method)}

	leaf := rq.req
	leaf.Auth = sess.EffectiveAuth(rq.id)

	envSnap := sess.SnapshotActiveEnvVars()
	ctrl := &runControl{stop: stop} // backs helena.runner for this request (#92)
	exec := headlessExecutor{
		sess:       sess,
		client:     client,
		rt:         scripting.New(envBridge{s: sess, base: envSnap}),
		globalSnap: sess.SnapshotGlobalVars(),
		dotEnvSnap: sess.SnapshotActiveDotEnvVars(),
		colSnap:    sess.SnapshotActiveCollectionVars(),
		envSnap:    envSnap,
		ctrl:       ctrl,
	}

	// The snapshot deep-copies the whole collection and chain.Resolve never
	// consults the finder when the chain is empty — skip it for chainless
	// requests (most of a typical collection during a full run).
	var finder chain.RequestFinder = nilFinder{}
	if len(leaf.Chain) > 0 {
		if snap := sess.SnapshotChainFinder(); snap != nil {
			finder = snap
		}
	}

	preOverlay := sess.SnapshotEnvOverlay()
	defer sess.RestoreEnvOverlay(preOverlay)

	chainMap, _, chainErr := chain.Resolve(ctx, leaf, finder, exec, nil)
	if chainErr != nil {
		res.Err = "chain: " + chainErr.Error()
		return res
	}

	// Chain predecessors share ctrl (so their stop() works); clear any skip they
	// set so it can't carry into the leaf — only the leaf's own pre-script skips.
	ctrl.resetSkip()
	view, tests, err := exec.executeOnce(ctx, leaf, chainMap, true)
	if ctrl.skipped() {
		res.Skipped = true
		for _, t := range tests {
			res.Checks = append(res.Checks, Check{Name: t.Name, Passed: t.Passed, Error: t.Error})
		}
		return res
	}
	res.URL = view.Request.URL
	res.StatusCode = view.Response.StatusCode
	res.Duration = view.Response.Duration
	if err != nil && view.Response.StatusCode == 0 {
		res.Err = err.Error()
		return res
	}
	if err != nil {
		res.Err = "post-script: " + err.Error()
	}

	for _, t := range tests {
		res.Checks = append(res.Checks, Check{Name: t.Name, Passed: t.Passed, Error: t.Error})
	}
	for _, a := range assertion.Evaluate(leaf.Assertions, view.Response.StatusCode, view.Response.Headers, view.Response.Body) {
		res.Checks = append(res.Checks, Check{Name: a.Name, Passed: a.Passed, Error: a.Error})
	}
	return res
}
