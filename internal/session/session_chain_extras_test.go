package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestSnapshotChainFinderFlattensAuth verifies that a request loaded
// with the implicit AuthInherit (no auth block in YAML) gets its Auth
// pre-resolved against the parent folder's auth when accessed via the
// snapshot finder. Without this, chain steps fire unauthenticated even
// when the user expects folder inheritance.
func TestSnapshotChainFinderFlattensAuth(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth",
			Auth: model.Auth{
				Type:   model.AuthBearer,
				Bearer: &model.BearerAuth{Token: "folder-token"},
			},
			Requests: []model.Request{{Name: "Login", Method: model.POST, URL: "https://api/login"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	finder := s.SnapshotChainFinder()
	if finder == nil {
		t.Fatal("SnapshotChainFinder returned nil with a loaded collection")
	}
	got, ok := finder.FindRequestByPath("Auth/Login")
	if !ok {
		t.Fatal("snapshot finder didn't find Auth/Login")
	}
	if got.Auth.Type != model.AuthBearer {
		t.Errorf("Auth.Type = %q, want Bearer (inherited from folder)", got.Auth.Type)
	}
	if got.Auth.Bearer == nil || got.Auth.Bearer.Token != "folder-token" {
		t.Errorf("Auth.Bearer = %+v, want token=folder-token", got.Auth.Bearer)
	}
}

// TestSnapshotChainFinderIsolatesFromLiveTree verifies that mutating
// the session's active collection after taking a snapshot does NOT
// affect the snapshot finder's results — proves no aliasing.
func TestSnapshotChainFinderIsolatesFromLiveTree(t *testing.T) {
	c := model.Collection{
		Name:     "Demo",
		Auth:     model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{Name: "Ping", Method: model.GET, URL: "https://x/"}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	_ = s.OpenCollection(dir)

	snap := s.SnapshotChainFinder()

	// Mutate the live tree after the snapshot.
	s.cols[0].Requests[0].Name = "RenamedPing"

	// Snapshot still resolves the old name.
	if _, ok := snap.FindRequestByPath("Ping"); !ok {
		t.Error("snapshot lost the original Ping after live rename")
	}
	if _, ok := snap.FindRequestByPath("RenamedPing"); ok {
		t.Error("snapshot saw the post-rename name; expected isolation")
	}
}

// TestSnapshotChainFinderFiltersPathSegments verifies the snapshot
// finder applies the same path-segment filtering as the live method
// (empty / "." / ".." dropped).
func TestSnapshotChainFinderFiltersPathSegments(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{{Name: "Login", Method: model.POST, URL: "https://api/login"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	_ = s.OpenCollection(dir)

	snap := s.SnapshotChainFinder()
	for _, p := range []string{"Auth/Login", "Auth/Login/", "/Auth/Login", "Auth//Login", "/Auth//Login/"} {
		if _, ok := snap.FindRequestByPath(p); !ok {
			t.Errorf("path %q: not found, expected match", p)
		}
	}
	for _, p := range []string{"Auth/../Login", "../Login", "."} {
		if _, ok := snap.FindRequestByPath(p); ok {
			t.Errorf("path %q: matched, expected miss after segment filter", p)
		}
	}
}

// TestRenameRequestCascadesChainRefs verifies that renaming a chained
// request rewrites every other request's ChainStep.Request that
// pointed to it. The most common shape (Profile chains Login, user
// renames Login → SignIn) should keep working without re-editing
// Profile's chain.
func TestRenameRequestCascadesChainRefs(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{{Name: "Login", Method: model.POST, URL: "https://api/login"}},
		}},
		Requests: []model.Request{{
			Name: "Profile", Method: model.GET, URL: "https://api/profile",
			Chain: []model.ChainStep{{Alias: "login", Request: "Auth/Login"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	_ = s.OpenCollection(dir)

	// Locate the Login request's tree node ID.
	loginID := "0/f0/r0"
	if _, ok := s.Tree().Request(loginID); !ok {
		t.Fatalf("can't find Login at %s", loginID)
	}
	if err := s.RenameItem(loginID, "SignIn"); err != nil {
		t.Fatalf("RenameItem: %v", err)
	}

	// Reload from disk to assert persistence too.
	c2, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c2.Requests[0].Chain[0].Request != "Auth/SignIn" {
		t.Errorf("chain ref = %q, want 'Auth/SignIn'", c2.Requests[0].Chain[0].Request)
	}
}

// TestRenameFolderCascadesChainRefs verifies the folder-rename case:
// renaming "Auth" → "Authentication" rewrites every ChainStep ref
// whose path went through that folder.
func TestRenameFolderCascadesChainRefs(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{{Name: "Login", Method: model.POST, URL: "https://api/login"}},
		}},
		Requests: []model.Request{{
			Name: "Profile", Method: model.GET, URL: "https://api/profile",
			Chain: []model.ChainStep{{Alias: "login", Request: "Auth/Login"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	_ = s.OpenCollection(dir)

	folderID := "0/f0"
	if err := s.RenameItem(folderID, "Authentication"); err != nil {
		t.Fatalf("RenameItem: %v", err)
	}
	c2, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c2.Requests[0].Chain[0].Request != "Authentication/Login" {
		t.Errorf("chain ref = %q, want 'Authentication/Login'", c2.Requests[0].Chain[0].Request)
	}
}

// TestRestoreEnvOverlay verifies that snapshot + restore round-trips
// the overlay map: writes between the two land go away.
func TestRestoreEnvOverlay(t *testing.T) {
	s, _ := New("")
	s.SetEnvOverlay("KEPT", "yes")
	snap := s.SnapshotEnvOverlay()

	s.SetEnvOverlay("TRANSIENT", "yes")
	s.SetEnvOverlay("KEPT", "modified")

	s.RestoreEnvOverlay(snap)

	if v, _ := s.EnvOverlay("KEPT"); v != "yes" {
		t.Errorf("KEPT after restore = %q, want yes", v)
	}
	if _, ok := s.EnvOverlay("TRANSIENT"); ok {
		t.Errorf("TRANSIENT survived restore, expected drop")
	}
}
