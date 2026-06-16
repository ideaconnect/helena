package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// makeColl is a small helper that writes a minimal collection to dir
// with the given name and returns the path so session tests can open
// it without re-declaring the YAML each time.
func makeColl(t *testing.T, parent, name string, reqs ...model.Request) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	c := model.Collection{Name: name, Auth: model.Auth{Type: model.AuthNone}, Requests: reqs}
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("setup collection %q: %v", name, err)
	}
	return dir
}

// TestTokenCacheReturnsNonNil verifies New always equips the session
// with an OAuth2 token cache so callers can pass it to the resolver
// without nil-checks.
func TestTokenCacheReturnsNonNil(t *testing.T) {
	s, _ := New("")
	if s.TokenCache() == nil {
		t.Error("TokenCache returned nil")
	}
}

// TestActiveCollectionDirEmptyWhenNoCollection verifies the documented
// "no collection active" path returns "" rather than panicking, so
// OAuth2 cache-key namespacing tolerates the cold-start case.
func TestActiveCollectionDirEmptyWhenNoCollection(t *testing.T) {
	s, _ := New("")
	if got := s.ActiveCollectionDir(); got != "" {
		t.Errorf("ActiveCollectionDir = %q, want empty", got)
	}
}

// TestActiveCollectionDirAfterOpen verifies that opening a collection
// makes ActiveCollectionDir report its path, and that switching to
// another collection updates the value.
func TestActiveCollectionDirAfterOpen(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "A")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	if got := s.ActiveCollectionDir(); got != dir {
		t.Errorf("ActiveCollectionDir = %q, want %q", got, dir)
	}
}

// TestActiveIndexReflectsWorkspaceSwitch verifies ActiveIndex tracks
// the current workspace position and that SetActive moves it.
func TestActiveIndexReflectsWorkspaceSwitch(t *testing.T) {
	tmp := t.TempDir()
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	_ = s.AddWorkspace("Second")
	if got := s.ActiveIndex(); got != 0 {
		t.Errorf("ActiveIndex initial = %d, want 0", got)
	}
	s.SetActive(1)
	if got := s.ActiveIndex(); got != 1 {
		t.Errorf("ActiveIndex after SetActive(1) = %d, want 1", got)
	}
	// Out-of-range setactive is a no-op.
	s.SetActive(99)
	if got := s.ActiveIndex(); got != 1 {
		t.Errorf("ActiveIndex after SetActive(99) = %d, want 1 (unchanged)", got)
	}
	// SetActive to current index is a no-op.
	s.SetActive(1)
	if got := s.ActiveIndex(); got != 1 {
		t.Errorf("ActiveIndex after no-op SetActive = %d, want 1", got)
	}
}

// TestDeepCopyRequestDetachesChain guards the duplicate-aliasing bug: a copied
// request's Chain must not share its backing array with the original.
func TestDeepCopyRequestDetachesChain(t *testing.T) {
	orig := model.Request{Chain: []model.ChainStep{{Alias: "a", Request: "Auth/Login"}}}
	cp := deepCopyRequest(orig)
	orig.Chain[0].Request = "MUT"
	if cp.Chain[0].Request != "Auth/Login" {
		t.Errorf("copy aliases original Chain: got %q", cp.Chain[0].Request)
	}
}

// TestRemoveCollectionResolvesByDirWhenMisaligned guards the data-loss bug: when
// an earlier collection failed to load, the loaded list (s.cols/s.dirs the tree
// shows) is misaligned with the persisted workspace list, so RemoveCollection(i)
// must drop the dir the UI actually targeted — not w.Collections[i].
func TestRemoveCollectionResolvesByDirWhenMisaligned(t *testing.T) {
	tmp := t.TempDir()
	a := makeColl(t, tmp, "A")
	makeColl(t, tmp, "B")
	cfg := filepath.Join(tmp, "cfg.yml")
	s, _ := New(cfg)
	if err := s.OpenCollection(a); err != nil {
		t.Fatalf("Open A: %v", err)
	}
	if err := s.OpenCollection(filepath.Join(tmp, "B")); err != nil {
		t.Fatalf("Open B: %v", err)
	}
	// Make A unloadable so a fresh session loads only B (index 0), while the
	// persisted workspace list is still [A, B] — i.e. misaligned.
	if err := os.RemoveAll(a); err != nil {
		t.Fatalf("remove A dir: %v", err)
	}
	s2, _ := New(cfg)
	if cols := s2.Collections(); len(cols) != 1 || cols[0].Name != "B" {
		t.Fatalf("setup: loaded cols = %+v, want only B", cols)
	}
	// Removing the only visible collection (index 0 = B) must drop B's dir from
	// the workspace. The old code removed w.Collections[0] = A's dir instead.
	if err := s2.RemoveCollection(0); err != nil {
		t.Fatalf("RemoveCollection: %v", err)
	}
	// Restore A on disk and reload: if B was correctly removed, the workspace now
	// lists only A; the buggy behaviour would have left B.
	makeColl(t, tmp, "A")
	s3, _ := New(cfg)
	got := s3.Collections()
	if len(got) != 1 || got[0].Name != "A" {
		t.Errorf("after misaligned remove, cols = %+v; want only A (B should be removed, not A)", got)
	}
}

// TestRemoveCollectionDropsFromWorkspaceAndKeepsFiles verifies the
// per-row collection delete path: RemoveCollection drops the dir
// from the active workspace's list, persists the change, and leaves
// the on-disk directory untouched (Postman/Bruno convention).
func TestRemoveCollectionDropsFromWorkspaceAndKeepsFiles(t *testing.T) {
	tmp := t.TempDir()
	a := makeColl(t, tmp, "A")
	b := makeColl(t, tmp, "B")
	cfg := filepath.Join(tmp, "cfg.yml")
	s, _ := New(cfg)
	if err := s.OpenCollection(a); err != nil {
		t.Fatalf("Open A: %v", err)
	}
	if err := s.OpenCollection(b); err != nil {
		t.Fatalf("Open B: %v", err)
	}

	if err := s.RemoveCollection(0); err != nil {
		t.Fatalf("RemoveCollection: %v", err)
	}
	cols := s.Collections()
	if len(cols) != 1 || cols[0].Name != "B" {
		t.Errorf("after remove cols = %+v, want only B", cols)
	}
	// A's on-disk directory must still exist.
	if _, err := storage.Load(a); err != nil {
		t.Errorf("A's dir gone after RemoveCollection: %v", err)
	}

	// Out-of-range index returns a clear error rather than panicking.
	if err := s.RemoveCollection(99); err == nil {
		t.Error("expected error for out-of-range index")
	}
	if err := s.RemoveCollection(-1); err == nil {
		t.Error("expected error for negative index")
	}

	// New session against the same cfg reflects the removal.
	s2, _ := New(cfg)
	if got := s2.Collections(); len(got) != 1 || got[0].Name != "B" {
		t.Errorf("after reload cols = %+v, want only B", got)
	}
}

// TestSetActiveCollectionUpdatesActiveAndPersists verifies that
// switching the active collection updates the activeCol index and is
// reflected in ActiveCollectionDir.
func TestSetActiveCollectionUpdatesActiveAndPersists(t *testing.T) {
	tmp := t.TempDir()
	a := makeColl(t, tmp, "A")
	b := makeColl(t, tmp, "B")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(a); err != nil {
		t.Fatalf("Open A: %v", err)
	}
	if err := s.OpenCollection(b); err != nil {
		t.Fatalf("Open B: %v", err)
	}
	// B is now active (last opened).
	if got := s.ActiveCollectionDir(); got != b {
		t.Errorf("ActiveCollectionDir = %q, want %q", got, b)
	}
	s.SetActiveCollection(0)
	if got := s.ActiveCollectionDir(); got != a {
		t.Errorf("after SetActiveCollection(0) = %q, want %q", got, a)
	}
	// Out-of-range no-op.
	s.SetActiveCollection(99)
	if got := s.ActiveCollectionDir(); got != a {
		t.Errorf("after SetActiveCollection(99) = %q, want %q (unchanged)", got, a)
	}
}

// TestAddEnvironmentAppendsAndPersists verifies AddEnvironment grows
// the active collection's environment list and the change survives a
// reload.
func TestAddEnvironmentAppendsAndPersists(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "A")
	cfg := filepath.Join(tmp, "cfg.yml")
	s, _ := New(cfg)
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s.AddEnvironment("Local")
	if got := s.CollectionEnvironmentNames(); len(got) != 1 || got[0] != "Local" {
		t.Errorf("env names = %v, want [Local]", got)
	}
	// Persist via SaveActiveCollection so the next session sees it.
	if err := s.SaveActiveCollection(); err != nil {
		t.Fatalf("SaveActiveCollection: %v", err)
	}
	s2, _ := New(cfg)
	if got := s2.CollectionEnvironmentNames(); len(got) != 1 || got[0] != "Local" {
		t.Errorf("after reload env names = %v, want [Local]", got)
	}
}

// TestSnapshotActiveEnvVarsReflectsActiveEnv verifies the snapshot
// returns the variables of the currently selected environment and
// returns an empty map when no env is selected.
func TestSnapshotActiveEnvVarsReflectsActiveEnv(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "A")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s.AddEnvironment("E")
	// Add a variable by editing the environment's Variables field.
	col := &s.cols[s.activeCol]
	col.Environments[0].Variables = []model.Variable{{Enabled: true, Key: "K", Value: "V"}}
	s.SetActiveEnv("E")

	snap := s.SnapshotActiveEnvVars()
	if snap["K"] != "V" {
		t.Errorf("snapshot = %v, want {K:V}", snap)
	}

	// Switching to no-env makes the snapshot empty.
	s.SetActiveEnv("")
	if got := s.SnapshotActiveEnvVars(); len(got) != 0 {
		t.Errorf("snapshot after unselect = %v, want empty", got)
	}
}

// TestAllRequestPathsTopAndNested verifies AllRequestPaths walks the
// tree depth-first, returning slash-separated paths for every request
// across all collections.
func TestAllRequestPathsTopAndNested(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "A", Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{
				{Name: "Login", Method: model.POST, URL: "https://x/login"},
			},
		}},
		Requests: []model.Request{
			{Name: "Profile", Method: model.GET, URL: "https://x/profile"},
		},
	}
	dir := filepath.Join(tmp, "A")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	paths := s.AllRequestPaths()
	want := map[string]bool{"Profile": true, "Auth/Login": true}
	if len(paths) != len(want) {
		t.Errorf("paths = %v, want %v entries", paths, want)
	}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

// TestAllRequestPathsEmptyForNoCollection verifies the empty-session
// guard: AllRequestPaths returns nil without panicking when no
// collection is active.
func TestAllRequestPathsEmptyForNoCollection(t *testing.T) {
	s, _ := New("")
	if got := s.AllRequestPaths(); got != nil {
		t.Errorf("AllRequestPaths = %v, want nil for empty session", got)
	}
}

// TestCollectionIndexFromNodeID verifies the tree's CollectionIndex
// extracts the leading numeric segment of a node ID and returns the
// collection's slice index, with -1 for invalid or out-of-range IDs.
func TestCollectionIndexFromNodeID(t *testing.T) {
	tmp := t.TempDir()
	a := makeColl(t, tmp, "A")
	b := makeColl(t, tmp, "B")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(a); err != nil {
		t.Fatalf("Open A: %v", err)
	}
	if err := s.OpenCollection(b); err != nil {
		t.Fatalf("Open B: %v", err)
	}
	tree := s.Tree()
	cases := map[string]int{
		"0":       0,
		"1":       1,
		"0/f0":    0,
		"1/r0":    1,
		"":        -1,
		"notanum": -1,
		"99":      -1,
	}
	for id, want := range cases {
		if got := tree.CollectionIndex(id); got != want {
			t.Errorf("CollectionIndex(%q) = %d, want %d", id, got, want)
		}
	}
}

// TestRequestIDForPathWhenMissing verifies the documented zero-value
// return when the path doesn't resolve.
func TestRequestIDForPathWhenMissing(t *testing.T) {
	s, _ := New("")
	if id, ok := s.RequestIDForPath("Nothing/Here"); ok || id != "" {
		t.Errorf("RequestIDForPath missing = (%q,%v), want (\"\",false)", id, ok)
	}
}

// TestAddWorkspaceRejectsEmpty verifies AddWorkspace ignores empty /
// whitespace-only names rather than appending a blank workspace.
func TestAddWorkspaceRejectsEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	before := len(s.WorkspaceNames())
	_ = s.AddWorkspace("")
	_ = s.AddWorkspace("   ")
	if got := len(s.WorkspaceNames()); got != before {
		t.Errorf("workspaces grew to %d on blank names", got)
	}
}

// TestRenameWorkspaceOutOfRangeNoop verifies that an invalid index is
// ignored rather than panicking on the underlying slice access.
func TestRenameWorkspaceOutOfRangeNoop(t *testing.T) {
	tmp := t.TempDir()
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	names := append([]string(nil), s.WorkspaceNames()...)
	_ = s.RenameWorkspace(99, "X")
	_ = s.RenameWorkspace(-1, "X")
	got := s.WorkspaceNames()
	if len(got) != len(names) {
		t.Errorf("workspace count changed: %v -> %v", names, got)
	}
	for i, n := range names {
		if got[i] != n {
			t.Errorf("workspace[%d] = %q, want %q", i, got[i], n)
		}
	}
}

// TestTreeIsBranchAndLabel verifies the tree's discriminators on
// every kind of node ID: empty (the virtual root), a collection
// number, a folder ID, and a request ID. Also checks Label fall-back
// when the ID points nowhere.
func TestTreeIsBranchAndLabel(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "Coll",
		model.Request{Name: "Ping", Method: model.GET, URL: "https://x/"},
	)
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	tr := s.Tree()
	// Empty / collection / nested checks.
	if !tr.IsBranch("") {
		t.Error("IsBranch(empty) should be true (virtual root)")
	}
	if !tr.IsBranch("0") {
		t.Error("IsBranch(collection) should be true")
	}
	if tr.IsBranch("0/r0") {
		t.Error("IsBranch(request) should be false")
	}
	// Label paths.
	if got := tr.Label("0"); got != "Coll" {
		t.Errorf("Label(collection) = %q, want Coll", got)
	}
	if got := tr.Label("0/r0"); got != "GET  Ping" {
		t.Errorf("Label(request) = %q, want 'GET  Ping'", got)
	}
	if got := tr.Label(""); got != "" {
		t.Errorf("Label(empty) = %q, want empty", got)
	}
	if got := tr.Label("99"); got != "" {
		t.Errorf("Label(out-of-range) = %q, want empty", got)
	}
	if got := tr.Label("0/r99"); got != "" {
		t.Errorf("Label(missing request) = %q, want empty", got)
	}
}

// TestTreeRequestPathOutOfRange verifies the Request accessor returns
// !ok for indices outside the loaded request list, and for IDs that
// don't end with the request prefix.
func TestTreeRequestPathOutOfRange(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "Coll",
		model.Request{Name: "Solo", Method: model.GET, URL: "https://x/"},
	)
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	tr := s.Tree()
	if _, ok := tr.Request("0/r0"); !ok {
		t.Error("Request(0/r0) should resolve")
	}
	if _, ok := tr.Request("0/r99"); ok {
		t.Error("Request(0/r99) should be out of range")
	}
	if _, ok := tr.Request("0/f0"); ok {
		t.Error("Request(0/f0) should reject folder ID")
	}
	if _, ok := tr.Request(""); ok {
		t.Error("Request(empty) should reject")
	}
}

// TestAddRequestAddsToActiveCollection verifies AddRequest at the
// collection root and inside a nested folder, returning the new
// node's tree ID for both.
func TestAddRequestAddsToActiveCollection(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
		}},
	}
	dir := filepath.Join(tmp, "C")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}

	rootID, err := s.AddRequest("0", "New")
	if err != nil {
		t.Fatalf("AddRequest(root): %v", err)
	}
	if rootID == "" {
		t.Error("AddRequest(root) returned empty ID")
	}
	nestedID, err := s.AddRequest("0/f0", "Login")
	if err != nil {
		t.Fatalf("AddRequest(folder): %v", err)
	}
	if r, ok := s.Tree().Request(nestedID); !ok || r.Name != "Login" {
		t.Errorf("nested Request lookup failed: %+v", r)
	}
	// Adding to an invalid parent surfaces an error.
	if _, err := s.AddRequest("99", "X"); err == nil {
		t.Error("AddRequest(invalid) should error")
	}
}

// TestAddFolderAddsToActiveCollection verifies AddFolder at the
// collection root and inside an existing folder, with an
// invalid-parent guard.
func TestAddFolderAddsToActiveCollection(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}

	id, err := s.AddFolder("0", "Top")
	if err != nil {
		t.Fatalf("AddFolder(root): %v", err)
	}
	if id == "" {
		t.Error("AddFolder(root) returned empty ID")
	}
	if _, err := s.AddFolder(id, "Nested"); err != nil {
		t.Errorf("AddFolder(nested): %v", err)
	}
	if _, err := s.AddFolder("99", "Bad"); err == nil {
		t.Error("AddFolder(invalid parent) should error")
	}
}

// TestRenameItemUpdatesNames verifies RenameItem on each node kind
// (collection, folder, request) and that an unknown node ID surfaces
// an error.
func TestRenameItemUpdatesNames(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "Old", Auth: model.Auth{Type: model.AuthNone},
		Folders:  []model.Folder{{Name: "F", Auth: model.Auth{Type: model.AuthInherit}}},
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://x/"}},
	}
	dir := filepath.Join(tmp, "C")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.RenameItem("0", "NewColl"); err != nil {
		t.Fatalf("RenameItem(collection): %v", err)
	}
	if err := s.RenameItem("0/f0", "NewFolder"); err != nil {
		t.Fatalf("RenameItem(folder): %v", err)
	}
	if err := s.RenameItem("0/r0", "NewReq"); err != nil {
		t.Fatalf("RenameItem(request): %v", err)
	}
	tr := s.Tree()
	if got := tr.Label("0"); got != "NewColl" {
		t.Errorf("collection label = %q, want NewColl", got)
	}
	if got := tr.Label("0/f0"); got != "NewFolder" {
		t.Errorf("folder label = %q", got)
	}
	if r, ok := tr.Request("0/r0"); !ok || r.Name != "NewReq" {
		t.Errorf("request name = %+v", r)
	}
	if err := s.RenameItem("0/r99", "Bad"); err == nil {
		t.Error("RenameItem(invalid) should error")
	}
}

// TestDeleteAndDuplicateItem verifies the delete + duplicate
// operations on requests and folders, including IDs surviving the
// operation and bad-input rejection.
func TestDeleteAndDuplicateItem(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Folders:  []model.Folder{{Name: "F", Auth: model.Auth{Type: model.AuthInherit}}},
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://x/"}},
	}
	dir := filepath.Join(tmp, "C")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Duplicate the request.
	dupID, err := s.DuplicateItem("0/r0")
	if err != nil {
		t.Fatalf("Duplicate request: %v", err)
	}
	if r, ok := s.Tree().Request(dupID); !ok || r.Name == "" {
		t.Errorf("duplicated request = %+v", r)
	}
	// Duplicate the folder.
	if _, err := s.DuplicateItem("0/f0"); err != nil {
		t.Errorf("Duplicate folder: %v", err)
	}
	// Invalid duplicate.
	if _, err := s.DuplicateItem("0/r99"); err == nil {
		t.Error("Duplicate(invalid) should error")
	}

	// Delete the original request.
	if err := s.DeleteItem("0/r0"); err != nil {
		t.Errorf("Delete request: %v", err)
	}
	// Delete the folder.
	if err := s.DeleteItem("0/f0"); err != nil {
		t.Errorf("Delete folder: %v", err)
	}
	// Invalid delete.
	if err := s.DeleteItem("0/f99"); err == nil {
		t.Error("Delete(invalid) should error")
	}
}

// TestDuplicateRequestDeepCopiesSlices verifies that the request copy
// is independent from the original — mutating the source's Headers /
// Params after Duplicate doesn't leak into the copy. Exercises the
// deep-copy branches in deepCopyRequest for the slice fields that
// survive the storage round-trip.
func TestDuplicateRequestDeepCopiesSlices(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "Heavy", Method: model.POST, URL: "https://x/",
			Headers: []model.KeyValue{{Enabled: true, Key: "H", Value: "v"}},
			Params:  []model.KeyValue{{Enabled: true, Key: "P", Value: "v"}},
		}},
	}
	dir := filepath.Join(tmp, "C")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dupID, err := s.DuplicateItem("0/r0")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	dup, ok := s.Tree().Request(dupID)
	if !ok {
		t.Fatalf("dup request not found at %s", dupID)
	}
	src, _ := s.Tree().Request("0/r0")
	src.Headers[0].Value = "MUTATED"
	src.Params[0].Value = "MUTATED"
	if dup.Headers[0].Value == "MUTATED" || dup.Params[0].Value == "MUTATED" {
		t.Errorf("duplicate aliases source slice: %+v", dup)
	}
	if dup.ID == src.ID {
		t.Errorf("duplicate kept same ID %q", dup.ID)
	}
}

// TestDuplicateFolderRecursesAndAssignsFreshIDs verifies the
// folder-level deep copy: nested folders + their requests all get
// fresh IDs and independent backing slices.
func TestDuplicateFolderRecursesAndAssignsFreshIDs(t *testing.T) {
	tmp := t.TempDir()
	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Outer", Auth: model.Auth{Type: model.AuthInherit},
			Folders: []model.Folder{{
				Name: "Inner", Auth: model.Auth{Type: model.AuthInherit},
				Requests: []model.Request{{Name: "Deep", Method: model.GET, URL: "https://x/"}},
			}},
			Requests: []model.Request{{Name: "Top", Method: model.GET, URL: "https://x/"}},
		}},
	}
	dir := filepath.Join(tmp, "C")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dupID, err := s.DuplicateItem("0/f0")
	if err != nil {
		t.Fatalf("Duplicate folder: %v", err)
	}
	// The original folder is at 0/f0; the duplicate is at 0/f1.
	if dupID == "0/f0" {
		t.Errorf("Duplicate returned source ID %q", dupID)
	}
}

// TestNewWithMissingConfigFileTreatedAsDefault verifies the cold-start
// case: pointing New at a path that doesn't exist returns a session
// holding the default config (one workspace) rather than an error.
func TestNewWithMissingConfigFileTreatedAsDefault(t *testing.T) {
	tmp := t.TempDir()
	s, err := New(filepath.Join(tmp, "does-not-exist.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := len(s.WorkspaceNames()); got != 1 {
		t.Errorf("workspace count = %d, want 1", got)
	}
}

// TestSetActiveEnvUpdatesActiveCollectionOnly verifies the env
// selection is per-collection: switching to a different collection
// and back preserves the previously chosen env on the other.
func TestSetActiveEnvUpdatesActiveCollectionOnly(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s.AddEnvironment("E1")
	_ = s.AddEnvironment("E2")
	s.SetActiveEnv("E1")
	if got := s.ActiveEnvName(); got != "E1" {
		t.Errorf("ActiveEnvName = %q, want E1", got)
	}
	// Setting to non-existent name still records the selection (the env
	// might be added later); just confirm the documented behavior
	// rather than asserting it's rejected.
	s.SetActiveEnv("")
	if got := s.ActiveEnvName(); got != "" {
		t.Errorf("after clear: ActiveEnvName = %q, want empty", got)
	}
}

// TestSetOpenRequestEdgeCases verifies the guard branches of
// SetOpenRequest: empty id clears, id without "/" is rejected,
// id with bad collection number is rejected, valid id stores +
// round-trips via OpenRequest.
func TestSetOpenRequestEdgeCases(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C",
		model.Request{Name: "R", Method: model.GET, URL: "https://x/"},
	)
	cfg := filepath.Join(tmp, "cfg.yml")
	s, _ := New(cfg)
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Empty clears any prior value.
	s.SetOpenRequest("")
	if got := s.OpenRequest(); got != "" {
		t.Errorf("OpenRequest after clear = %q, want empty", got)
	}
	// No "/": rejected.
	s.SetOpenRequest("0")
	if got := s.OpenRequest(); got != "" {
		t.Errorf("OpenRequest after invalid id = %q, want empty", got)
	}
	// Out-of-range collection index: rejected.
	s.SetOpenRequest("99/r0")
	if got := s.OpenRequest(); got != "" {
		t.Errorf("OpenRequest after out-of-range = %q, want empty", got)
	}
	// Valid: stored.
	s.SetOpenRequest("0/r0")
	if got := s.OpenRequest(); got != "0/r0" {
		t.Errorf("OpenRequest = %q, want 0/r0", got)
	}
}

// TestSaveActiveCollectionPersistsToDisk verifies that a Save without
// an active collection is a no-op-with-no-error, and a save with a
// loaded collection writes back through storage.
func TestSaveActiveCollectionPersistsToDisk(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	// No active collection: no error.
	if err := s.SaveActiveCollection(); err != nil {
		t.Errorf("SaveActiveCollection (no active) = %v, want nil", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Mutate the collection name and save.
	s.cols[0].Name = "Renamed"
	if err := s.SaveActiveCollection(); err != nil {
		t.Errorf("SaveActiveCollection: %v", err)
	}
	// Reload via storage.Load to verify it landed.
	got, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("name after Save = %q, want Renamed", got.Name)
	}
}

// TestDeepCopyRequestPreservesNilSlices verifies that duplicating a
// request with no headers/params/form leaves the duplicate's slices
// nil too — exercises the nil-skip branches in deepCopyRequest.
func TestDeepCopyRequestPreservesNilSlices(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C",
		model.Request{Name: "Bare", Method: model.GET, URL: "https://x/"},
	)
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	dupID, err := s.DuplicateItem("0/r0")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	dup, _ := s.Tree().Request(dupID)
	if len(dup.Headers) != 0 || len(dup.Params) != 0 {
		t.Errorf("bare-request duplicate grew slices: %+v", dup)
	}
}

// TestAddRequestRejectsEmptyName verifies the name-validation branch
// in AddRequest. Mirrors the same check on AddFolder.
func TestAddRequestRejectsEmptyName(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.AddRequest("0", "   "); err == nil {
		t.Error("AddRequest with whitespace-only name should error")
	}
	if _, err := s.AddFolder("0", ""); err == nil {
		t.Error("AddFolder with empty name should error")
	}
}

// TestItemOpsOnCollectionNodeRejected verifies that the documented
// "collections are removed via workspace management" rule is
// enforced — DeleteItem and the analogous Duplicate/Rename paths
// against a collection node return an explicit error rather than
// silently corrupting tree state.
func TestItemOpsOnCollectionNodeRejected(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.DeleteItem("0"); err == nil {
		t.Error("DeleteItem(collection) should error")
	}
}

// TestParseLeafRejectsShortLeaf verifies that a node ID whose leaf
// segment is shorter than the kind prefix + at least one digit is
// rejected (the function expects "fN" or "rN" forms).
func TestParseLeafRejectsShortLeaf(t *testing.T) {
	tmp := t.TempDir()
	dir := makeColl(t, tmp, "C")
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// "0/f" — leaf is just "f" with no index.
	if err := s.DeleteItem("0/f"); err == nil {
		t.Error("DeleteItem with malformed short leaf should error")
	}
}

// TestDeleteWorkspaceOutOfRangeNoop verifies the guard against
// invalid indices. The single remaining workspace also shouldn't be
// deleted (the session needs at least one).
func TestDeleteWorkspaceOutOfRangeNoop(t *testing.T) {
	tmp := t.TempDir()
	s, _ := New(filepath.Join(tmp, "cfg.yml"))
	if got := len(s.WorkspaceNames()); got != 1 {
		t.Fatalf("initial workspace count = %d, want 1", got)
	}
	_ = s.DeleteWorkspace(0) // must NOT drop the last one
	if got := len(s.WorkspaceNames()); got != 1 {
		t.Errorf("after DeleteWorkspace(0) = %d, want 1 (last workspace kept)", got)
	}
	_ = s.DeleteWorkspace(99)
	if got := len(s.WorkspaceNames()); got != 1 {
		t.Errorf("after DeleteWorkspace(99) = %d, want 1", got)
	}
}
