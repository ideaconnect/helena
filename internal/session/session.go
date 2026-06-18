// Package session ties persisted config to collections loaded from disk and
// exposes them for the UI: workspace switching, a tree navigation model, the
// active collection/environment used to resolve {{variables}}, and restorable
// UI state (open request, window size, …).
package session

import (
	"fmt"
	"slices"
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
	loadErrs  []LoadError        // collections of the active workspace that failed to load
	overlayMu sync.RWMutex
	overlay   map[string]string // script-set env; in-memory only, never persisted
}

// LoadError records a collection directory in the active workspace that failed
// to load during reload, together with the underlying error. The UI surfaces
// these so a corrupt or moved collection does not silently disappear from the
// sidebar with no explanation.
type LoadError struct {
	Dir string
	Err error
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

// CollectionDir returns the on-disk directory of the loaded collection at
// index i, or "" when i is out of range. Used by the UI to record which
// collection an open tab belongs to (a stable key across collection
// reordering, unlike the index).
func (s *Session) CollectionDir(i int) string {
	if i < 0 || i >= len(s.dirs) {
		return ""
	}
	return s.dirs[i]
}

// LoadErrors returns the collections of the active workspace that failed to
// load on the most recent reload (empty when all loaded). The slice is a copy,
// so callers may retain it across further reloads. The UI surfaces these as a
// non-transient diagnostic so a failed collection is never silently dropped.
func (s *Session) LoadErrors() []LoadError {
	if len(s.loadErrs) == 0 {
		return nil
	}
	return slices.Clone(s.loadErrs)
}

// reload re-reads the collections of the active workspace from disk and
// rebuilds the per-collection active-environment map. A collection that fails
// to load is dropped from the in-memory set (so the misalignment-safe indexing
// downstream can't mis-target it) but its error is captured in s.loadErrs so
// the UI can surface a diagnostic rather than letting it silently disappear.
func (s *Session) reload() {
	s.cols = nil
	s.dirs = nil
	s.loadErrs = nil
	for _, dir := range s.activeWorkspace().Collections {
		c, err := storage.Load(dir)
		if err != nil {
			s.loadErrs = append(s.loadErrs, LoadError{Dir: dir, Err: err})
			continue
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
	// Already open in this workspace? Re-activate the existing entry instead of
	// loading a second in-memory copy that would race the same on-disk files
	// (last-save-wins data loss).
	if i := slices.Index(s.dirs, dir); i >= 0 {
		s.activeCol = i
		s.cfg.UI.ActiveCollection = dir
		return s.persist()
	}
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

// RemoveCollection removes the collection at index i from the active
// workspace's collections list. The on-disk directory is left
// untouched — this is purely a "remove from this workspace" action,
// matching the Postman/Bruno convention. Out-of-range i is a no-op
// returning a clear error.
func (s *Session) RemoveCollection(i int) error {
	if s.cfg.Active < 0 || s.cfg.Active >= len(s.cfg.Workspaces) {
		return fmt.Errorf("no active workspace")
	}
	// i indexes the loaded collections (s.cols/s.dirs) the tree renders, which
	// can be a subset/reordering of the persisted dir list when some failed to
	// load. Resolve the target by dir and delete *that* entry — never
	// w.Collections[i], which drops the wrong collection when misaligned.
	if i < 0 || i >= len(s.dirs) {
		return fmt.Errorf("collection index %d out of range", i)
	}
	w := &s.cfg.Workspaces[s.cfg.Active]
	dir := s.dirs[i]
	if j := slices.Index(w.Collections, dir); j >= 0 {
		w.Collections = slices.Delete(w.Collections, j, j+1)
	}
	// If the active collection was the one removed, clear the UI state pointer
	// so reload doesn't try to re-select a missing dir.
	if s.cfg.UI.ActiveCollection == dir {
		s.cfg.UI.ActiveCollection = ""
	}
	s.reload()
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
	if s.activeCol < 0 || s.activeCol >= len(s.dirs) {
		return // no valid active collection — don't pollute the env map with a -1 key
	}
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

// environmentIndex returns the index of the named environment in the active
// collection, or -1.
func (s *Session) environmentIndex(name string) int {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return -1
	}
	for i, e := range s.cols[s.activeCol].Environments {
		if e.Name == name {
			return i
		}
	}
	return -1
}

// AddEnvironment appends a new uniquely-named environment to the active
// collection and persists. Returns an error if no collection is active, the
// name is empty, or an environment with that name already exists.
func (s *Session) AddEnvironment(name string) error {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return fmt.Errorf("no active collection")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("environment name cannot be empty")
	}
	if s.environmentIndex(name) >= 0 {
		return fmt.Errorf("an environment named %q already exists", name)
	}
	s.cols[s.activeCol].Environments = append(s.cols[s.activeCol].Environments,
		model.Environment{ID: model.NewID(), Name: name})
	return s.SaveActiveCollection()
}

// RenameEnvironment renames an environment in the active collection and
// persists. The new name must be non-empty and not collide with another
// environment; if the renamed environment was active, the active selection
// follows it.
func (s *Session) RenameEnvironment(oldName, newName string) error {
	idx := s.environmentIndex(oldName)
	if idx < 0 {
		return fmt.Errorf("environment %q not found", oldName)
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("environment name cannot be empty")
	}
	if newName != oldName && s.environmentIndex(newName) >= 0 {
		return fmt.Errorf("an environment named %q already exists", newName)
	}
	s.cols[s.activeCol].Environments[idx].Name = newName
	if s.ActiveEnvName() == oldName {
		s.SetActiveEnv(newName)
	}
	return s.SaveActiveCollection()
}

// DeleteEnvironment removes an environment from the active collection and
// persists. If it was the active environment, the active selection moves to the
// first remaining one (or clears when none remain).
func (s *Session) DeleteEnvironment(name string) error {
	idx := s.environmentIndex(name)
	if idx < 0 {
		return fmt.Errorf("environment %q not found", name)
	}
	wasActive := s.ActiveEnvName() == name
	s.cols[s.activeCol].Environments = slices.Delete(s.cols[s.activeCol].Environments, idx, idx+1)
	if wasActive {
		if names := s.CollectionEnvironmentNames(); len(names) > 0 {
			s.SetActiveEnv(names[0])
		} else {
			s.SetActiveEnv("")
		}
	}
	return s.SaveActiveCollection()
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
	return vars.New(s.activeEnvVars(), s.SnapshotEnvOverlay()).WithFallback(vars.Dynamic)
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

// LocateRequest finds the request carrying the persistent Request.ID
// requestID inside the loaded collection whose source directory is dir,
// returning its current tree node ID and a live pointer into s.cols. The
// lookup is scoped to the owning collection (by dir) so two collections
// that share a Request.ID — e.g. a collection forked on disk with both
// copies opened — never resolve to each other's requests. The returned
// node ID is freshly derived from the current tree shape, so callers can
// rely on it after add/delete/duplicate shifts sibling indices. Returns
// ("", nil, false) when requestID is empty, dir is not loaded, or no
// request in that collection carries the ID.
func (s *Session) LocateRequest(dir, requestID string) (string, *model.Request, bool) {
	if requestID == "" {
		return "", nil, false
	}
	for ci := range s.dirs {
		if s.dirs[ci] != dir {
			continue
		}
		col := &s.cols[ci]
		return locateInContainer(strconv.Itoa(ci), col.Folders, col.Requests, requestID)
	}
	return "", nil, false
}

// locateInContainer searches requests then nested folders for a request
// whose ID matches, building the tree node ID from prefix as it descends.
// The returned *model.Request points into the passed slices (live data in
// s.cols when the caller threads s.cols through).
func locateInContainer(prefix string, folders []model.Folder, requests []model.Request, id string) (string, *model.Request, bool) {
	for i := range requests {
		if requests[i].ID == id {
			return fmt.Sprintf("%s/r%d", prefix, i), &requests[i], true
		}
	}
	for i := range folders {
		if nodeID, req, ok := locateInContainer(fmt.Sprintf("%s/f%d", prefix, i), folders[i].Folders, folders[i].Requests, id); ok {
			return nodeID, req, true
		}
	}
	return "", nil, false
}

// ContainerRef is one selectable destination container — a collection
// root or a folder — for the "Save As" picker: a human-readable Label and
// the tree NodeID a new request would be added under.
type ContainerRef struct {
	Label  string
	NodeID string
}

// ContainerPaths lists every container (collection root + every folder)
// across all loaded collections in the active workspace, for the scratch
// "Save As" destination picker. Labels are collection-qualified
// ("MyAPI", "MyAPI / Auth", …) so duplicate folder names across
// collections stay distinguishable. Returns nil when nothing is loaded.
func (s *Session) ContainerPaths() []ContainerRef {
	var out []ContainerRef
	for ci := range s.cols {
		col := &s.cols[ci]
		nodeID := strconv.Itoa(ci)
		out = append(out, ContainerRef{Label: col.Name, NodeID: nodeID})
		collectContainerPaths(nodeID, col.Name, col.Folders, &out)
	}
	return out
}

func collectContainerPaths(prefix, label string, folders []model.Folder, out *[]ContainerRef) {
	for i := range folders {
		nodeID := fmt.Sprintf("%s/f%d", prefix, i)
		l := label + " / " + folders[i].Name
		*out = append(*out, ContainerRef{Label: l, NodeID: nodeID})
		collectContainerPaths(nodeID, l, folders[i].Folders, out)
	}
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
	return s.persistCollection(s.activeCol)
}

// saveCollection writes the collection at index ci to its source dir. Used by
// in-collection mutations (e.g. drag-reorder) that may target a collection
// other than the active one.
func (s *Session) saveCollection(ci int) error {
	return s.persistCollection(ci)
}

// persistCollection writes the collection at index ci to disk and, on failure,
// reloads the active workspace so the in-memory model matches what is actually
// stored. storage.Save is atomic (it stages then swaps), so a failed save
// leaves disk exactly as it was, and reload restores memory to that last-good
// state. This is the rollback guard for tree mutators (#109): a mutator that
// applies an in-memory change then fails to persist it never leaves memory
// diverged from disk. Because reload rebuilds the tree, any node IDs/pointers a
// caller computed before the failed save are invalid once an error is returned.
func (s *Session) persistCollection(ci int) error {
	if ci < 0 || ci >= len(s.cols) {
		return nil
	}
	if err := storage.Save(s.cols[ci], s.dirs[ci]); err != nil {
		s.reload()
		return err
	}
	return nil
}

// MoveCollection reorders the top-level collection list so the collection at
// index from ends up at index to (final-index semantics: removed from from,
// re-inserted at to clamped to the list length), then persists the new
// workspace order. The cols/dirs/workspace lists stay in lockstep and the
// active-collection pointer follows its collection. No-op when indices are
// equal or out of range.
func (s *Session) MoveCollection(from, to int) error {
	n := len(s.cols)
	if s.cfg.Active < 0 || s.cfg.Active >= len(s.cfg.Workspaces) {
		return fmt.Errorf("no active workspace")
	}
	w := &s.cfg.Workspaces[s.cfg.Active]
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return nil
	}
	if len(w.Collections) != n {
		// Misaligned (some collections failed to load); refuse rather than
		// scramble the persisted dir list.
		return fmt.Errorf("collection list out of sync; reopen the workspace")
	}

	// Remember the active collection by dir so we can restore its index after
	// the reorder shuffles positions.
	activeDir := ""
	if s.activeCol >= 0 && s.activeCol < len(s.dirs) {
		activeDir = s.dirs[s.activeCol]
	}

	col, dir, wc := s.cols[from], s.dirs[from], w.Collections[from]
	s.cols = slices.Delete(s.cols, from, from+1)
	s.dirs = slices.Delete(s.dirs, from, from+1)
	w.Collections = slices.Delete(w.Collections, from, from+1)

	ins := to
	if ins > len(s.cols) {
		ins = len(s.cols)
	}
	s.cols = slices.Insert(s.cols, ins, col)
	s.dirs = slices.Insert(s.dirs, ins, dir)
	w.Collections = slices.Insert(w.Collections, ins, wc)

	if activeDir != "" {
		for i, d := range s.dirs {
			if d == activeDir {
				s.activeCol = i
				break
			}
		}
	}
	return s.persist()
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

// SetOpenTabs records the open editor tabs (each by owning collection dir
// + persistent Request.ID) and which one is active, then persists once.
// The active index is clamped into range. The legacy single OpenRequest
// pointer is cleared because OpenTabs supersedes it — on the next launch
// the tab list is the source of truth. Passing an empty slice clears the
// tab state. Scratch tabs are not persistable and must be filtered out by
// the caller before calling this.
func (s *Session) SetOpenTabs(tabs []config.UIOpenTab, active int) {
	if len(tabs) == 0 {
		tabs = nil
		active = 0
	} else if active < 0 || active >= len(tabs) {
		active = 0
	}
	s.cfg.UI.OpenTabs = tabs
	s.cfg.UI.ActiveTab = active
	s.cfg.UI.OpenRequest = nil
	_ = s.persist()
}

// OpenTabs returns the persisted open tabs and the active-tab index. The
// caller resolves each tab against the loaded collections (via
// LocateRequest) and skips any that no longer resolve; the active index
// is clamped here to the persisted range but the caller must re-map it
// after dropping unresolvable tabs. Returns (nil, 0) when none persisted.
func (s *Session) OpenTabs() ([]config.UIOpenTab, int) {
	tabs := s.cfg.UI.OpenTabs
	if len(tabs) == 0 {
		return nil, 0
	}
	active := s.cfg.UI.ActiveTab
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return tabs, active
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
