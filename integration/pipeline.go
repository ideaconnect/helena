// Package integration hosts cross-package tests that exercise the
// wires between Helena's internal modules end-to-end without going
// through the UI. The package unit tests under internal/* verify
// the boxes; these tests verify the wires.
//
// Tests live alongside this shared helper (pipeline.go is non-test
// so every _test.go in the package can reuse the Sender / executor /
// env bridge without re-declaring them). The harness is deliberately
// thin — each test still sets up its own collection / server / state
// so failures point at one wire at a time.
//
// Coverage of this package is intentionally excluded from the 90%
// floor (see TESTING.md). What gates is whether the wires hold; not
// the lines of test infrastructure.
package integration

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
	"github.com/idct/helena/internal/vars"
)

// Pipeline bundles the moving parts of a Send through the internals:
// the on-disk collection dir, the Helena session, the test HTTP
// server, and the wired Send pipeline (httpclient + scripting +
// chain). Each test calls NewPipeline to get a fresh, isolated
// world; Close releases the server and tempdir.
type Pipeline struct {
	tb      testing.TB
	TmpDir  string
	CfgPath string
	CollDir string
	Sess    *session.Session
	Server  *httptest.Server
}

// NewPipeline creates a fresh pipeline rooted at a t.TempDir-scoped
// path. The session and the test server are wired but no collection
// is loaded yet — call OpenCollection / Save+Open as the test needs.
func NewPipeline(tb testing.TB, handler httptest.ResponseRecorder) *Pipeline {
	tb.Helper()
	tmp := tb.TempDir()
	p := &Pipeline{
		tb:      tb,
		TmpDir:  tmp,
		CfgPath: filepath.Join(tmp, "cfg.yml"),
		CollDir: filepath.Join(tmp, "collection"),
	}
	tb.Cleanup(p.Close)
	return p
}

// NewPipelineWithServer is the standard constructor: pass in a
// per-test httptest.Server so the test can register its handlers
// before construction. The pipeline stops the server in Cleanup.
func NewPipelineWithServer(tb testing.TB, server *httptest.Server) *Pipeline {
	tb.Helper()
	tmp := tb.TempDir()
	p := &Pipeline{
		tb:      tb,
		TmpDir:  tmp,
		CfgPath: filepath.Join(tmp, "cfg.yml"),
		CollDir: filepath.Join(tmp, "collection"),
		Server:  server,
	}
	tb.Cleanup(p.Close)
	return p
}

// Close releases the test server (if owned) and any other
// pipeline-scoped resources. Idempotent.
func (p *Pipeline) Close() {
	if p.Server != nil {
		p.Server.Close()
		p.Server = nil
	}
}

// SaveAndOpen writes the collection to CollDir and opens it as the
// active collection on a fresh session at CfgPath. Use this as the
// canonical setup for tests that start with a model.Collection.
func (p *Pipeline) SaveAndOpen(c model.Collection) error {
	if err := storage.Save(c, p.CollDir); err != nil {
		return err
	}
	s, err := session.New(p.CfgPath)
	if err != nil {
		return err
	}
	if err := s.OpenCollection(p.CollDir); err != nil {
		return err
	}
	p.Sess = s
	return nil
}

// Reopen drops the current session and creates a fresh one at
// CfgPath, then opens CollDir. Mirrors what a user pressing the
// Workspace dropdown would trigger (cold-start the session against
// existing on-disk state).
func (p *Pipeline) Reopen() error {
	p.Sess = nil
	s, err := session.New(p.CfgPath)
	if err != nil {
		return err
	}
	if err := s.OpenCollection(p.CollDir); err != nil {
		return err
	}
	p.Sess = s
	return nil
}

// Send runs the request at chain-ref path through the full pipeline
// (chain → leaf), the same path the UI Send button takes. Returns
// the captured response, the accumulated console output, and the
// first error from any phase. If chain resolution fails the leaf is
// not executed (the documented contract); the env overlay is rolled
// back to its pre-Send snapshot.
func (p *Pipeline) Send(reqPath string) (*chain.View, []string, error) {
	if p.Sess == nil {
		return nil, nil, errSessNotOpen
	}
	leaf, ok := p.Sess.FindRequestByPath(reqPath)
	if !ok {
		return nil, nil, errRequestNotFound(reqPath)
	}
	// Flatten Inherit auth via the same path the UI takes.
	if id := nodeIDFor(p.Sess, reqPath); id != "" {
		leaf.Auth = p.Sess.EffectiveAuth(id)
	}

	envSnap := p.Sess.SnapshotActiveEnvVars()
	client := httpclient.New(model.DefaultSettings())
	client.SetOAuth2Resolver(auth.NewClientCredentialsResolver(
		p.Sess.TokenCache(), nil, p.Sess.ActiveCollectionDir()))
	rt := scripting.New(envBridge{s: p.Sess, base: envSnap})
	exec := chainExecutor{sess: p.Sess, client: client, rt: rt, envSnap: envSnap}

	var finder chain.RequestFinder
	if snap := p.Sess.SnapshotChainFinder(); snap != nil {
		finder = snap
	} else {
		finder = nilFinder{}
	}

	preOverlay := p.Sess.SnapshotEnvOverlay()
	ctx := context.Background()
	chainMap, chainConsole, chainErr := chain.Resolve(ctx, leaf, finder, exec, nil)
	if chainErr != nil {
		p.Sess.RestoreEnvOverlay(preOverlay)
		return nil, chainConsole, chainErr
	}
	view, leafConsole, leafErr := exec.ExecuteOnce(ctx, leaf, chainMap)
	console := append(chainConsole, leafConsole...)
	if leafErr != nil && view.Response.StatusCode == 0 {
		return nil, console, leafErr
	}
	return &view, console, leafErr
}

// envBridge is the scripting.EnvBridge wired to the pipeline's
// session — script reads layer the captured snapshot on top of the
// live overlay; writes hit the overlay only.
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

// chainExecutor is the same shape as the UI's chainExecutor — runs
// pre → http → post and packages the response as a chain.View. Kept
// here so integration tests don't pull in internal/ui.
type chainExecutor struct {
	sess    *session.Session
	client  *httpclient.Client
	rt      *scripting.Runtime
	envSnap map[string]string
}

func (e chainExecutor) ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []string, error) {
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)

	scriptChain := chainViewToScripting(chainMap)
	var console []string

	preRes, preErr := e.rt.RunPreRequest(ctx, r.Scripts.PreRequest, &r, scriptChain)
	console = append(console, preRes.Console...)
	if preErr != nil {
		return chain.View{}, console, preErr
	}
	resolver := vars.New(e.envSnap, e.sess.SnapshotEnvOverlay()).WithFallback(chain.VarLookup(chainMap))
	resp, err := e.client.Do(ctx, r, resolver)
	if err != nil {
		return chain.View{}, console, err
	}
	view := chain.View{
		Request: chain.RequestView{
			Method: string(r.Method), URL: resp.RequestURL, Body: resp.RequestBody,
		},
		Response: chain.ResponseView{
			StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers,
			Body: resp.Body, Size: resp.Size, Duration: resp.Duration, CORSWarning: resp.CORSWarning,
		},
	}
	postRes, postErr := e.rt.RunPostResponse(ctx, r.Scripts.PostResponse, r,
		scripting.ResponseInput{
			StatusCode: resp.StatusCode, Status: resp.Status,
			Headers: resp.Headers, Body: resp.Body,
		}, scriptChain)
	console = append(console, postRes.Console...)
	if postErr != nil {
		return view, console, postErr
	}
	return view, console, nil
}

// chainViewToScripting converts the chain runner's View map into the
// scripting binding's ChainView map — same shape, different
// packages by design (AGENTS invariant 15).
func chainViewToScripting(in map[string]chain.View) map[string]scripting.ChainView {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]scripting.ChainView, len(in))
	for alias, v := range in {
		out[alias] = scripting.ChainView{
			Request: scripting.ChainRequestView{
				Method: v.Request.Method, URL: v.Request.URL, Body: v.Request.Body,
			},
			Response: scripting.ResponseInput{
				StatusCode: v.Response.StatusCode, Status: v.Response.Status,
				Headers: v.Response.Headers, Body: v.Response.Body,
			},
		}
	}
	return out
}

// nilFinder mirrors the UI's cold-start finder: every lookup returns
// !ok so chain.Resolve still walks the leaf's (empty) chain without
// crashing when no collection is loaded.
type nilFinder struct{}

func (nilFinder) FindRequestByPath(string) (model.Request, bool) { return model.Request{}, false }
func (nilFinder) FindRequestByID(string) (model.Request, bool)   { return model.Request{}, false }

// nodeIDFor returns the tree node ID of the request at chain-ref
// path, or "" if not found. Used to flatten Inherit auth before
// Send so the chain runner sees the resolved auth.
func nodeIDFor(s *session.Session, path string) string {
	for _, id := range s.Tree().ChildIDs("") {
		if got := walkTree(s.Tree(), id, "", path); got != "" {
			return got
		}
	}
	return ""
}

// walkTree mirrors features/world.go's walkTreeForNode: skips the
// collection-level label (chain-ref paths are collection-relative).
func walkTree(tree *session.Tree, nodeID, prefix, target string) string {
	if r, ok := tree.Request(nodeID); ok {
		candidate := prefix
		if candidate != "" {
			candidate += "/"
		}
		candidate += r.Name
		if candidate == target {
			return nodeID
		}
		return ""
	}
	nextPrefix := prefix
	if containsSlash(nodeID) { // folder (not collection root)
		label := tree.Label(nodeID)
		if label != "" {
			if nextPrefix != "" {
				nextPrefix += "/"
			}
			nextPrefix += label
		}
	}
	for _, child := range tree.ChildIDs(nodeID) {
		if got := walkTree(tree, child, nextPrefix, target); got != "" {
			return got
		}
	}
	return ""
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

var (
	errSessNotOpen     = pipelineError("integration: no session opened — call SaveAndOpen first")
	errRequestNotFound = func(p string) error {
		return pipelineError("integration: request not found: " + p)
	}
)

type pipelineError string

func (e pipelineError) Error() string { return string(e) }
