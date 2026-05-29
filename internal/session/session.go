// Package session ties persisted config to collections loaded from disk and
// exposes them for the UI: workspace switching, a tree navigation model, the
// active collection/environment used to resolve {{variables}}, and restorable
// UI state (open request, window size, …).
package session

import (
	"strconv"
	"strings"
	"sync"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
	"github.com/idct/helena/internal/vars"
)

// Session is the in-memory application state for the active workspace.
type Session struct {
	cfgPath   string
	cfg       config.Config
	cols      []model.Collection // collections loaded for the active workspace
	dirs      []string           // source directory of each loaded collection, aligned with cols
	activeCol int                // index into cols, or -1 when none
	activeEnv map[int]string     // collection index -> active environment name
	tokens    *auth.TokenCache   // OAuth2 access tokens cached for this session
	overlayMu sync.RWMutex
	overlay   map[string]string // script-set env; in-memory only, never persisted
}

// New loads the config at cfgPath (empty path = defaults, no persistence) and
// the collections of the active workspace.
func New(cfgPath string) (*Session, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	s := &Session{cfgPath: cfgPath, cfg: cfg, tokens: auth.NewTokenCache(), overlay: map[string]string{}}
	s.reload()
	return s, nil
}

// TokenCache returns the OAuth2 token cache owned by this session. Tokens
// are keyed by collection-dir + auth config so two collections that
// happen to share a token URL never reuse each other's access tokens.
// The cache is process-lifetime only; tokens are dropped when Helena
// exits.
func (s *Session) TokenCache() *auth.TokenCache { return s.tokens }

// ActiveCollectionDir returns the on-disk directory of the active
// collection, or "" when no collection is active. Useful as a namespace
// prefix for OAuth2 token-cache keys.
func (s *Session) ActiveCollectionDir() string {
	if s.activeCol < 0 || s.activeCol >= len(s.dirs) {
		return ""
	}
	return s.dirs[s.activeCol]
}

// reload re-reads the collections of the active workspace from disk and
// rebuilds the per-collection active-environment map. Collections that fail to
// load are skipped silently — the UI surfaces those failures elsewhere.
func (s *Session) reload() {
	s.cols = nil
	s.dirs = nil
	for _, dir := range s.activeWorkspace().Collections {
		c, err := storage.Load(dir)
		if err != nil {
			continue // skip collections that no longer load; surfaced in UI later
		}
		s.cols = append(s.cols, c)
		s.dirs = append(s.dirs, dir)
	}

	// Restore active collection from persisted UI state, falling back to first.
	s.activeCol = -1
	if len(s.cols) > 0 {
		s.activeCol = 0
		if target := s.cfg.UI.ActiveCollection; target != "" {
			for i, d := range s.dirs {
				if d == target {
					s.activeCol = i
					break
				}
			}
		}
	}

	// Rebuild the per-collection active env map (index-keyed) from the
	// persisted path-keyed map.
	s.activeEnv = map[int]string{}
	for i, dir := range s.dirs {
		if name, ok := s.cfg.UI.ActiveEnv[dir]; ok {
			s.activeEnv[i] = name
		}
	}
}

// activeWorkspace returns the workspace at cfg.Active, or a zero workspace if
// the index is out of range.
func (s *Session) activeWorkspace() config.Workspace {
	if s.cfg.Active < 0 || s.cfg.Active >= len(s.cfg.Workspaces) {
		return config.Workspace{}
	}
	return s.cfg.Workspaces[s.cfg.Active]
}

// WorkspaceNames returns the names of all workspaces, in order.
func (s *Session) WorkspaceNames() []string {
	out := make([]string, len(s.cfg.Workspaces))
	for i, w := range s.cfg.Workspaces {
		out[i] = w.Name
	}
	return out
}

// ActiveIndex returns the index of the active workspace.
func (s *Session) ActiveIndex() int { return s.cfg.Active }

// SetActive switches the active workspace, reloads its collections, and persists.
func (s *Session) SetActive(i int) {
	if i < 0 || i >= len(s.cfg.Workspaces) || i == s.cfg.Active {
		return
	}
	s.cfg.Active = i
	s.reload()
	_ = s.persist()
}

// Collections returns the collections loaded for the active workspace.
func (s *Session) Collections() []model.Collection { return s.cols }

// OpenCollection loads an OpenCollection directory, adds it to the active
// workspace, makes it the active collection, and persists the change.
func (s *Session) OpenCollection(dir string) error {
	c, err := storage.Load(dir)
	if err != nil {
		return err
	}
	w := &s.cfg.Workspaces[s.cfg.Active]
	w.Collections = append(w.Collections, dir)
	s.cols = append(s.cols, c)
	s.dirs = append(s.dirs, dir)
	s.activeCol = len(s.cols) - 1
	s.cfg.UI.ActiveCollection = dir
	return s.persist()
}

// persist writes the current config to cfgPath. When cfgPath is empty (a
// transient in-memory session) it is a no-op.
func (s *Session) persist() error {
	if s.cfgPath == "" {
		return nil
	}
	return config.Save(s.cfgPath, s.cfg)
}

// Tree returns a navigation model over the currently loaded collections.
func (s *Session) Tree() *Tree { return &Tree{cols: s.cols} }

// EffectiveAuth flattens the Inherit chain for the request addressed by
// nodeID and returns the auth that should actually be applied. The
// request's own Auth wins outright when it is anything other than Inherit;
// otherwise the folder → collection ancestor chain is walked and the
// nearest non-Inherit value is used. Falls back to AuthNone when nothing
// concrete is set anywhere on the chain.
func (s *Session) EffectiveAuth(nodeID string) model.Auth {
	t := s.Tree()
	req, ok := t.Request(nodeID)
	var own model.Auth
	if ok {
		own = req.Auth
	}
	return auth.Resolve(own, t.AncestorAuths(nodeID))
}

// ActiveCollection returns the index of the active collection, or -1.
func (s *Session) ActiveCollection() int { return s.activeCol }

// SetActiveCollection sets which collection the environment selector and
// resolver apply to (-1 for none), and persists the choice.
func (s *Session) SetActiveCollection(i int) {
	if i < -1 || i >= len(s.cols) {
		return
	}
	s.activeCol = i
	if i >= 0 {
		s.cfg.UI.ActiveCollection = s.dirs[i]
	} else {
		s.cfg.UI.ActiveCollection = ""
	}
	_ = s.persist()
}

// CollectionEnvironmentNames lists the environment names of the active collection.
func (s *Session) CollectionEnvironmentNames() []string {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	envs := s.cols[s.activeCol].Environments
	out := make([]string, len(envs))
	for i, e := range envs {
		out[i] = e.Name
	}
	return out
}

// ActiveEnvName returns the active environment name for the active collection.
func (s *Session) ActiveEnvName() string {
	if s.activeEnv == nil {
		return ""
	}
	return s.activeEnv[s.activeCol]
}

// SetActiveEnv sets the active environment (by name) for the active collection,
// syncs the path-keyed persistence map, and saves.
func (s *Session) SetActiveEnv(name string) {
	if s.activeEnv == nil {
		s.activeEnv = map[int]string{}
	}
	s.activeEnv[s.activeCol] = name

	if s.cfg.UI.ActiveEnv == nil {
		s.cfg.UI.ActiveEnv = map[string]string{}
	}
	if s.activeCol >= 0 && s.activeCol < len(s.dirs) {
		dir := s.dirs[s.activeCol]
		if name == "" {
			delete(s.cfg.UI.ActiveEnv, dir)
		} else {
			s.cfg.UI.ActiveEnv[dir] = name
		}
	}
	_ = s.persist()
}

// ActiveEnvironment returns a pointer to the active environment, or nil.
func (s *Session) ActiveEnvironment() *model.Environment {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	name := s.ActiveEnvName()
	for i := range s.cols[s.activeCol].Environments {
		if s.cols[s.activeCol].Environments[i].Name == name {
			return &s.cols[s.activeCol].Environments[i]
		}
	}
	return nil
}

// AddEnvironment appends a new environment to the active collection.
func (s *Session) AddEnvironment(name string) {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return
	}
	s.cols[s.activeCol].Environments = append(s.cols[s.activeCol].Environments,
		model.Environment{ID: model.NewID(), Name: name})
}

// SetActiveEnvironmentVariables replaces the active environment's variables.
func (s *Session) SetActiveEnvironmentVariables(variables []model.Variable) {
	if e := s.ActiveEnvironment(); e != nil {
		e.Variables = variables
	}
}

// Resolver builds a variable resolver from the active collection's active
// environment (enabled variables only). The script-set env overlay is
// layered on top as the highest-precedence scope so a `helena.env.set(...)`
// during a pre-request or post-response hook is visible to the next Send
// without ever touching disk.
//
// Call from the UI goroutine. Workers should use the
// SnapshotActiveEnvVars + SnapshotEnvOverlay pair instead so the env
// can't shift mid-Send.
func (s *Session) Resolver() *vars.Resolver {
	return vars.New(s.activeEnvVars(), s.SnapshotEnvOverlay())
}

// SetEnvOverlay records a script-set environment variable for the lifetime
// of the process. Per the Helena scripting contract (AGENTS invariant 9),
// these never persist to the collection's environment file. Empty name is
// a no-op so scripts that accidentally call with a falsy key don't poison
// the overlay.
func (s *Session) SetEnvOverlay(name, value string) {
	if name == "" {
		return
	}
	s.overlayMu.Lock()
	if s.overlay == nil {
		s.overlay = map[string]string{}
	}
	s.overlay[name] = value
	s.overlayMu.Unlock()
}

// EnvOverlay returns the current value an overlay entry, or "" + false when
// nothing has been set under that name. The check covers callers that want
// to know whether the script explicitly set a value vs. inheriting from the
// underlying environment.
func (s *Session) EnvOverlay(name string) (string, bool) {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	v, ok := s.overlay[name]
	return v, ok
}

// ClearEnvOverlay removes every script-set entry. Useful between Send
// invocations when the user wants a clean slate without restarting Helena.
func (s *Session) ClearEnvOverlay() {
	s.overlayMu.Lock()
	s.overlay = map[string]string{}
	s.overlayMu.Unlock()
}

// RestoreEnvOverlay replaces the overlay with a copy of snap, dropping
// every entry not present there. The chain runner uses this with a
// SnapshotEnvOverlay() taken before chain.Resolve to undo writes a
// failing chain made — so a chain that succeeded partway through and
// then errored doesn't leak script-set values into the next Send.
func (s *Session) RestoreEnvOverlay(snap map[string]string) {
	s.overlayMu.Lock()
	s.overlay = make(map[string]string, len(snap))
	for k, v := range snap {
		s.overlay[k] = v
	}
	s.overlayMu.Unlock()
}

// SnapshotEnvOverlay returns a copy of the overlay so callers (the Send
// pipeline, Resolver scopes) don't race against concurrent
// SetEnvOverlay calls. Safe to call from any goroutine; the underlying
// map is locked under the overlay RWMutex.
func (s *Session) SnapshotEnvOverlay() map[string]string {
	s.overlayMu.RLock()
	defer s.overlayMu.RUnlock()
	out := make(map[string]string, len(s.overlay))
	for k, v := range s.overlay {
		out[k] = v
	}
	return out
}

// activeEnvVars returns the enabled key/value pairs of the active environment
// as a flat map, ready to feed into vars.Resolver.
func (s *Session) activeEnvVars() map[string]string {
	m := map[string]string{}
	e := s.ActiveEnvironment()
	if e == nil {
		return m
	}
	for _, v := range e.Variables {
		if v.Enabled {
			m[v.Key] = v.Value
		}
	}
	return m
}

// SnapshotActiveEnvVars returns a copy of the active environment's
// enabled variables. Called from the UI goroutine on Send entry so the
// worker goroutine can read env values without racing against later UI
// mutations to s.cols / s.activeCol / Environment.Variables. The
// returned map is owned by the caller.
func (s *Session) SnapshotActiveEnvVars() map[string]string {
	return s.activeEnvVars()
}

// FindRequestByPath walks the active collection for a request whose
// slash-separated Name path matches ref. `"Auth/Login"` resolves to the
// request named "Login" inside the folder named "Auth". A leading "/"
// is tolerated. Matching is case-sensitive on the display name. Returns
// (req, true) on the first match, or (zero, false) when nothing
// matches.
//
// Live-tree call: walks s.cols directly. Workers that run for the
// duration of a Send should use [Session.SnapshotChainFinder] instead
// so they don't race against UI-thread mutations of the active
// collection.
func (s *Session) FindRequestByPath(ref string) (model.Request, bool) {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return model.Request{}, false
	}
	parts := splitChainPath(ref)
	if len(parts) == 0 {
		return model.Request{}, false
	}
	col := &s.cols[s.activeCol]
	r, ok := findRequestInContainer(col.Folders, col.Requests, parts)
	if !ok {
		return model.Request{}, false
	}
	return cloneRequestForChain(r), true
}

// splitChainPath strips leading slashes, splits on "/", trims
// whitespace per segment, drops empty segments (so trailing or
// duplicated slashes are tolerated), and REJECTS the whole path if
// any segment is "." or ".." — chain refs use display names, not
// filesystem-style relatives, so those segments are always a sign of
// a malformed input. A rejected path returns nil so callers surface a
// clean miss instead of a surprising match.
func splitChainPath(ref string) []string {
	raw := strings.Split(strings.TrimPrefix(ref, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "." || p == ".." {
			return nil
		}
		out = append(out, p)
	}
	return out
}

// findRequestInContainer recurses into folders and matches against
// requests at the path's leaf. Empty path is an error case the caller
// rejects.
func findRequestInContainer(folders []model.Folder, requests []model.Request, parts []string) (model.Request, bool) {
	if len(parts) == 0 {
		return model.Request{}, false
	}
	if len(parts) == 1 {
		for _, r := range requests {
			if r.Name == parts[0] {
				return r, true
			}
		}
		return model.Request{}, false
	}
	head, rest := parts[0], parts[1:]
	for _, f := range folders {
		if f.Name == head {
			if r, ok := findRequestInContainer(f.Folders, f.Requests, rest); ok {
				return r, true
			}
		}
	}
	return model.Request{}, false
}

// cloneRequestForChain deep-copies the slice-backed fields of a
// Request returned from the active-collection walk so the worker
// goroutine can't race against UI-thread edits to the live collection
// (params/headers row writes, body.Form edits, chain-step edits).
// Struct-level fields are value-copied by the `r model.Request`
// signature already.
func cloneRequestForChain(r model.Request) model.Request {
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)
	r.Chain = append([]model.ChainStep(nil), r.Chain...)
	return r
}

// AllRequestPaths returns every chain-ref-style request path within
// the active collection. Used by the UI's Chain tab to populate an
// autocomplete dropdown so users get suggestions instead of typos
// surfacing as runtime errors. Paths are returned in tree order;
// folders alone are not emitted (only their leaf requests). Returns
// nil when no collection is loaded.
func (s *Session) AllRequestPaths() []string {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	col := &s.cols[s.activeCol]
	var out []string
	collectRequestPaths(col.Folders, col.Requests, "", &out)
	return out
}

func collectRequestPaths(folders []model.Folder, requests []model.Request, prefix string, out *[]string) {
	for _, r := range requests {
		*out = append(*out, joinChainSegment(prefix, r.Name))
	}
	for _, f := range folders {
		collectRequestPaths(f.Folders, f.Requests, joinChainSegment(prefix, f.Name), out)
	}
}

func joinChainSegment(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// RequestIDForPath looks up the persistent Request.ID of the request at
// chain-ref path `ref` in the active collection. Used by the UI's
// Chain tab so picking a path from autocomplete also pins the chain
// step's RequestID — the resulting reference then survives renames or
// folder moves of the target. Returns ("", false) when no collection
// is loaded, the path is empty, or the path doesn't resolve.
func (s *Session) RequestIDForPath(ref string) (string, bool) {
	r, ok := s.FindRequestByPath(ref)
	if !ok {
		return "", false
	}
	return r.ID, true
}

// SnapshotChainFinder returns a snapshot of the active collection
// reusable from the worker goroutine: it owns its own copies of the
// folders/requests trees plus every request's slice-backed fields, and
// pre-flattens each request's Auth via the same ancestor walk Send
// uses for the leaf. Returns nil when no collection is loaded — the
// caller should treat that as "no chain steps resolvable".
//
// The returned finder is safe to use concurrently with arbitrary
// UI-thread mutations of the live Session because it doesn't reach
// back into s.cols at all. Capture once at Send entry, hand to
// chain.Resolve.
func (s *Session) SnapshotChainFinder() *ChainFinderSnapshot {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	col := s.cols[s.activeCol]
	snap := &ChainFinderSnapshot{
		folders:  cloneFoldersWithAuth(col.Folders, []model.Auth{col.Auth}),
		requests: cloneRequestsWithAuth(col.Requests, []model.Auth{col.Auth}),
		byID:     map[string]model.Request{},
	}
	indexRequestsByID(snap.byID, snap.folders, snap.requests)
	return snap
}

// indexRequestsByID populates dst with id → Request entries for every
// request in the cloned snapshot tree. Empty IDs are skipped; the
// chain runner treats them as "no pinned ID" and uses the path
// fallback. Used only at SnapshotChainFinder construction so the
// snapshot owns a self-contained ID lookup.
func indexRequestsByID(dst map[string]model.Request, folders []model.Folder, requests []model.Request) {
	for _, r := range requests {
		if r.ID != "" {
			dst[r.ID] = r
		}
	}
	for _, f := range folders {
		indexRequestsByID(dst, f.Folders, f.Requests)
	}
}

// ChainFinderSnapshot satisfies chain.RequestFinder and owns a deep
// copy of the active collection at construction time. It pre-resolves
// each request's Auth via auth.Resolve(own, ancestors) so chain steps
// inherit the same way the leaf does (the Send-time leaf flattening at
// shell.go uses EffectiveAuth; this snapshot does the same walk per
// request). byID is built alongside the cloned tree so chain steps
// pinned with RequestID resolve in O(1) without walking the tree.
type ChainFinderSnapshot struct {
	folders  []model.Folder
	requests []model.Request
	byID     map[string]model.Request
}

// FindRequestByPath is the chain.RequestFinder implementation. Uses
// the same splitChainPath rules as the live Session method.
func (f *ChainFinderSnapshot) FindRequestByPath(ref string) (model.Request, bool) {
	if f == nil {
		return model.Request{}, false
	}
	parts := splitChainPath(ref)
	if len(parts) == 0 {
		return model.Request{}, false
	}
	return findRequestInContainer(f.folders, f.requests, parts)
}

// FindRequestByID is the chain.RequestFinder by-ID implementation. The
// id map was built once at snapshot construction so this is constant
// time and never reaches back into the live session state.
func (f *ChainFinderSnapshot) FindRequestByID(id string) (model.Request, bool) {
	if f == nil || id == "" {
		return model.Request{}, false
	}
	r, ok := f.byID[id]
	return r, ok
}

// cloneFoldersWithAuth deep-copies the folder tree and pre-flattens
// each descendant request's Auth via auth.Resolve against the
// ancestor chain. ancestors is the outer→inner Auth list of every
// container above this level (collection root first, then enclosing
// folders, then this folder itself).
func cloneFoldersWithAuth(in []model.Folder, ancestors []model.Auth) []model.Folder {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.Folder, len(in))
	for i, f := range in {
		// Push this folder's auth as the new innermost ancestor for
		// its descendants.
		child := append(append([]model.Auth(nil), ancestors...), f.Auth)
		out[i] = model.Folder{
			ID:       f.ID,
			Name:     f.Name,
			Auth:     f.Auth,
			Folders:  cloneFoldersWithAuth(f.Folders, child),
			Requests: cloneRequestsWithAuth(f.Requests, child),
		}
	}
	return out
}

// cloneRequestsWithAuth deep-copies the request list and replaces
// each request's Auth with the ancestor-resolved Auth so the chain
// runner gets a request whose Auth is already flat (Basic/Bearer/etc
// or None — never Inherit).
func cloneRequestsWithAuth(in []model.Request, ancestors []model.Auth) []model.Request {
	if len(in) == 0 {
		return nil
	}
	// Walk ancestors innermost-first for auth.Resolve, which expects
	// inner-first order.
	rev := make([]model.Auth, len(ancestors))
	for i, a := range ancestors {
		rev[len(ancestors)-1-i] = a
	}
	out := make([]model.Request, len(in))
	for i, r := range in {
		r = cloneRequestForChain(r)
		r.Auth = auth.Resolve(r.Auth, rev)
		out[i] = r
	}
	return out
}

// SaveActiveCollection writes the active collection back to its source directory.
func (s *Session) SaveActiveCollection() error {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	return storage.Save(s.cols[s.activeCol], s.dirs[s.activeCol])
}

// Settings returns the current application settings.
func (s *Session) Settings() model.Settings { return s.cfg.Settings }

// SetSettings replaces the application settings and persists them.
func (s *Session) SetSettings(st model.Settings) {
	s.cfg.Settings = st
	_ = s.persist()
}

// SetOpenRequest remembers the currently open request by collection path + the
// in-collection node path. Empty id clears it. Persists.
func (s *Session) SetOpenRequest(nodeID string) {
	if nodeID == "" {
		s.cfg.UI.OpenRequest = nil
		_ = s.persist()
		return
	}
	idx := strings.IndexByte(nodeID, '/')
	if idx < 0 {
		return
	}
	ci, err := strconv.Atoi(nodeID[:idx])
	if err != nil || ci < 0 || ci >= len(s.dirs) {
		return
	}
	s.cfg.UI.OpenRequest = &config.UIOpenRequest{
		Collection: s.dirs[ci],
		NodePath:   nodeID[idx+1:],
	}
	_ = s.persist()
}

// OpenRequest reconstructs the persisted open-request node ID against the
// currently loaded collections, or returns "" if it can't be resolved.
func (s *Session) OpenRequest() string {
	or := s.cfg.UI.OpenRequest
	if or == nil {
		return ""
	}
	for i, d := range s.dirs {
		if d == or.Collection {
			return strconv.Itoa(i) + "/" + or.NodePath
		}
	}
	return ""
}

// SetWindowSize stores the window size for restoration on next launch.
func (s *Session) SetWindowSize(w, h int) {
	s.cfg.UI.WindowWidth = w
	s.cfg.UI.WindowHeight = h
	_ = s.persist()
}

// WindowSize returns the persisted window size, or (0, 0) if unset.
func (s *Session) WindowSize() (int, int) {
	return s.cfg.UI.WindowWidth, s.cfg.UI.WindowHeight
}
