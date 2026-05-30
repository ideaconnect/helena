package features

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
	"github.com/idct/helena/internal/vars"
)

// world is the per-scenario state shared across step definitions. It
// owns the on-disk fixtures, the Helena session, the test HTTP
// server, and the captured result of the last Send. Each scenario
// gets a fresh world via the Before hook so steps don't leak across
// scenarios.
type world struct {
	tmpDir      string // unique per scenario
	cfgPath     string // session config path inside tmpDir
	collDir     string // active collection directory inside tmpDir
	sess        *session.Session
	server      *httptest.Server
	mux         *handlerMux
	client      *httpclient.Client
	rt          *scripting.Runtime
	envSnap     map[string]string
	exec        chainExecutor
	finder      chain.RequestFinder
	resp        *httpclient.Response
	lastErr     error
	console     []string
	chainMap    map[string]chain.View
	capturedID  string            // captured Request.ID for persistence scenarios
	captureVars map[string]string // free-form capture slot for misc scenario state
	curl        string            // last cURL rendering for import_export scenarios
}

// newWorld constructs a fresh world with a unique on-disk workspace.
// Caller must invoke close in the After hook.
func newWorld() (*world, error) {
	tmp, err := os.MkdirTemp("", "helena-features-*")
	if err != nil {
		return nil, err
	}
	w := &world{
		tmpDir:      tmp,
		cfgPath:     filepath.Join(tmp, "cfg.yml"),
		collDir:     filepath.Join(tmp, "collection"),
		mux:         newHandlerMux(),
		captureVars: map[string]string{},
	}
	w.server = httptest.NewServer(w.mux)
	return w, nil
}

func (w *world) close() {
	if w.server != nil {
		w.server.Close()
	}
	if w.tmpDir != "" {
		_ = os.RemoveAll(w.tmpDir)
	}
}

// seedCollection writes the supplied collection to collDir and opens
// a session at cfgPath against it. The session's active collection is
// the seeded one.
func (w *world) seedCollection(c model.Collection) error {
	if err := storage.Save(c, w.collDir); err != nil {
		return err
	}
	s, err := session.New(w.cfgPath)
	if err != nil {
		return err
	}
	if err := s.OpenCollection(w.collDir); err != nil {
		return err
	}
	w.sess = s
	return nil
}

// ensureCollection guarantees w.sess holds an open session with an
// empty active collection. Idempotent — Given steps that incrementally
// build the collection call it before each addition so order of
// Givens doesn't matter.
func (w *world) ensureCollection() error {
	if w.sess != nil {
		return nil
	}
	return w.seedCollection(model.Collection{
		Name: "Feature",
		Auth: model.Auth{Type: model.AuthNone},
	})
}

// reopen drops the current session and opens a fresh one at cfgPath.
// Used by persistence scenarios to verify that disk state survives a
// session lifetime. The collection on disk at collDir stays untouched
// — the new session's reload picks it up via the persisted UI state.
func (w *world) reopen() error {
	w.sess = nil
	s, err := session.New(w.cfgPath)
	if err != nil {
		return err
	}
	w.sess = s
	return nil
}

// reopenWith creates a fresh session + opens the supplied collection
// directory as the active collection. Used after importer scenarios
// that just wrote a brand-new collection to disk and need the session
// to pick it up without relying on any persisted UI state.
func (w *world) reopenWith(dir string) error {
	w.sess = nil
	s, err := session.New(w.cfgPath)
	if err != nil {
		return err
	}
	if err := s.OpenCollection(dir); err != nil {
		return err
	}
	w.sess = s
	return nil
}

// liveRequest finds the model.Request at chain-ref path in the active
// collection and returns a mutable pointer. Used by incremental Given
// steps that need to add chain refs, scripts, or auth after the
// request was first added.
func (w *world) liveRequest(path string) *model.Request {
	if w.sess == nil {
		return nil
	}
	cols := w.sess.Collections()
	if len(cols) == 0 {
		return nil
	}
	c := &cols[0]
	parts := splitPath(path)
	folders := c.Folders
	requests := c.Requests
	for i, seg := range parts {
		if i == len(parts)-1 {
			for j := range requests {
				if requests[j].Name == seg {
					return &requests[j]
				}
			}
			return nil
		}
		var next *model.Folder
		for j := range folders {
			if folders[j].Name == seg {
				next = &folders[j]
				break
			}
		}
		if next == nil {
			return nil
		}
		folders = next.Folders
		requests = next.Requests
	}
	return nil
}

// liveFolder finds the model.Folder at the slash-separated path and
// returns a mutable pointer. Used by Given steps that set folder-level
// Auth.
func (w *world) liveFolder(path string) *model.Folder {
	if w.sess == nil {
		return nil
	}
	cols := w.sess.Collections()
	if len(cols) == 0 {
		return nil
	}
	folders := cols[0].Folders
	parts := splitPath(path)
	for i, seg := range parts {
		var next *model.Folder
		for j := range folders {
			if folders[j].Name == seg {
				next = &folders[j]
				break
			}
		}
		if next == nil {
			return nil
		}
		if i == len(parts)-1 {
			return next
		}
		folders = next.Folders
	}
	return nil
}

// nodeIDFor walks the active collection's tree looking for an item
// (request or folder) at chain-ref path. Returns "" if not found.
func (w *world) nodeIDFor(path string) string {
	if w.sess == nil {
		return ""
	}
	for _, id := range w.sess.Tree().ChildIDs("") {
		if got := walkTreeForNode(w.sess.Tree(), id, "", path); got != "" {
			return got
		}
	}
	return ""
}

// walkTreeForNode is like walkTreeForPath but also accepts a folder
// terminus — Rename targets folders too.
func walkTreeForNode(tree *session.Tree, nodeID, prefix, target string) string {
	label := tree.Label(nodeID)
	// Strip the "<METHOD>  " request prefix the Tree adds.
	if r, ok := tree.Request(nodeID); ok {
		label = r.Name
		candidate := prefix
		if candidate != "" {
			candidate += "/"
		}
		candidate += label
		if candidate == target {
			return nodeID
		}
		return ""
	}
	// Folder or collection.
	nextPrefix := prefix
	if label != "" && !isCollectionID(nodeID) {
		if nextPrefix != "" {
			nextPrefix += "/"
		}
		nextPrefix += label
		if nextPrefix == target {
			return nodeID
		}
	}
	for _, child := range tree.ChildIDs(nodeID) {
		if got := walkTreeForNode(tree, child, nextPrefix, target); got != "" {
			return got
		}
	}
	return ""
}

// isCollectionID reports whether id addresses a collection (no slash).
// Collections aren't part of chain-ref paths; only folders + requests
// are.
func isCollectionID(id string) bool { return !containsSlash(id) }

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

func splitPath(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// buildExec wires the same Send pipeline the UI shell uses: a
// chainExecutor backed by the session's environment snapshot + a
// fresh scripting runtime + httpclient. Called once per scenario
// before Send-style steps; reuses the seeded session.
func (w *world) buildExec() error {
	if w.sess == nil {
		return errors.New("buildExec: no session — call seedCollection first")
	}
	w.envSnap = w.sess.SnapshotActiveEnvVars()
	w.client = httpclient.New(model.DefaultSettings())
	w.client.SetOAuth2Resolver(auth.NewClientCredentialsResolver(
		w.sess.TokenCache(), nil, w.sess.ActiveCollectionDir()))
	w.rt = scripting.New(envBridge{s: w.sess, base: w.envSnap})
	w.exec = chainExecutor{w: w}
	if snap := w.sess.SnapshotChainFinder(); snap != nil {
		w.finder = snap
	} else {
		w.finder = nilFinder{}
	}
	return nil
}

// send runs the named request through the full pipeline (chain →
// leaf), captures the result on w.resp / w.lastErr / w.console for
// later assertions, and rolls back the env overlay on chain
// failure exactly like the UI Send path.
func (w *world) send(reqPath string) {
	w.resp = nil
	w.lastErr = nil
	w.console = nil
	w.chainMap = nil

	leaf, ok := w.sess.FindRequestByPath(reqPath)
	if !ok {
		w.lastErr = errors.New("request not found: " + reqPath)
		return
	}
	leaf.Auth = w.sess.EffectiveAuth(treeIDFor(w.sess, reqPath))

	if err := w.buildExec(); err != nil {
		w.lastErr = err
		return
	}
	ctx := context.Background()
	preOverlay := w.sess.SnapshotEnvOverlay()
	chainMap, chainConsole, chainErr := chain.Resolve(ctx, leaf, w.finder, w.exec, nil)
	if chainErr != nil {
		w.sess.RestoreEnvOverlay(preOverlay)
		w.lastErr = chainErr
		w.console = chainConsole
		return
	}
	w.chainMap = chainMap
	view, leafConsole, leafErr := w.exec.ExecuteOnce(ctx, leaf, chainMap)
	w.console = append(chainConsole, leafConsole...)
	if leafErr != nil && view.Response.StatusCode == 0 {
		w.lastErr = leafErr
		return
	}
	w.resp = &httpclient.Response{
		StatusCode:  view.Response.StatusCode,
		Status:      view.Response.Status,
		Headers:     view.Response.Headers,
		Body:        view.Response.Body,
		Size:        view.Response.Size,
		Duration:    view.Response.Duration,
		CORSWarning: view.Response.CORSWarning,
		RequestURL:  view.Request.URL,
		RequestBody: view.Request.Body,
	}
	if leafErr != nil {
		w.lastErr = leafErr // non-fatal post-script error
	}
}

// envBridge satisfies scripting.EnvBridge: reads layer through the
// captured env snapshot + the live overlay; writes go to the overlay
// (session-scoped, never persisted).
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

// chainExecutor mirrors the UI's chainExecutor — runs pre-script →
// httpclient.Do → post-script and packages the result as a
// chain.View. Decoupled from internal/ui so the feature suite can
// drive the same execution path without the Fyne dependency.
type chainExecutor struct {
	w *world
}

func (e chainExecutor) ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []string, error) {
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)

	scriptChain := chainViewToScripting(chainMap)
	var console []string

	preRes, preErr := e.w.rt.RunPreRequest(ctx, r.Scripts.PreRequest, &r, scriptChain)
	console = append(console, preRes.Console...)
	if preErr != nil {
		return chain.View{}, console, preErr
	}
	resolver := vars.New(e.w.envSnap, e.w.sess.SnapshotEnvOverlay())
	resp, err := e.w.client.Do(ctx, r, resolver)
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
	postRes, postErr := e.w.rt.RunPostResponse(ctx, r.Scripts.PostResponse, r,
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

// chainViewToScripting converts the chain.View map the chain runner
// uses into the scripting.ChainView map the goja bindings consume.
// Same shape, different package — the duplication is intentional
// (AGENTS invariant 15: chain stays free of scripting).
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

// nilFinder is the cold-start fallback for ChainFinder when no
// collection is loaded; mirrors the UI's nilFinder.
type nilFinder struct{}

func (nilFinder) FindRequestByPath(string) (model.Request, bool) { return model.Request{}, false }
func (nilFinder) FindRequestByID(string) (model.Request, bool)   { return model.Request{}, false }

// treeIDFor walks the active collection looking for a request whose
// chain-ref path matches `path`, returning its tree node ID for
// EffectiveAuth. Returns "" if no match — EffectiveAuth tolerates
// that.
//
// The walk uses walkTreeForNode (shared with nodeIDFor) which skips
// collection-level labels — chain-ref paths are relative to the
// active collection.
func treeIDFor(s *session.Session, path string) string {
	for _, id := range s.Tree().ChildIDs("") {
		if got := walkTreeForNode(s.Tree(), id, "", path); got != "" {
			return got
		}
	}
	return ""
}
