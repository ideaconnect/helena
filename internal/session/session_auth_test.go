package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// writeAuthSampleCollection writes a collection on disk where:
//   - the collection root has Bearer auth "root-token"
//   - folder "Users" has Basic auth alice/secret
//   - root request "Health" inherits
//   - folder request "Get Users" inherits
//   - folder request "Delete User" has its own API-Key auth
//
// Returns the directory holding the saved collection.
func writeAuthSampleCollection(t *testing.T) string {
	t.Helper()
	c := model.Collection{
		Name: "Auth Demo",
		Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "root-token"}},
		Requests: []model.Request{{
			Name:   "Health",
			Method: model.GET,
			URL:    "https://api/h",
			Auth:   model.Auth{Type: model.AuthInherit},
		}},
		Folders: []model.Folder{{
			Name: "Users",
			Auth: model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "alice", Password: "secret"}},
			Requests: []model.Request{
				{Name: "Get Users", Method: model.GET, URL: "https://api/u", Auth: model.Auth{Type: model.AuthInherit}},
				{Name: "Delete User", Method: model.DELETE, URL: "https://api/u/1",
					Auth: model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{
						Name: "X-Key", Value: "abc", Placement: model.APIKeyHeader,
					}}},
			},
		}},
	}
	dir := filepath.Join(t.TempDir(), "auth-demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

// newSessionWith opens dir as a collection in a fresh in-memory session.
func newSessionWith(t *testing.T, dir string) *Session {
	t.Helper()
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	return s
}

// TestEffectiveAuthFallsThroughFolderToCollection verifies that an
// Inherit-on-Inherit chain resolves all the way to the collection root.
func TestEffectiveAuthFallsThroughFolderToCollection(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	got := s.EffectiveAuth("0/f0/r0") // Users/Get Users
	if got.Type != model.AuthBasic || got.Basic == nil || got.Basic.Username != "alice" {
		t.Errorf("EffectiveAuth = %+v, want folder Basic", got)
	}
}

// TestEffectiveAuthCollectionRootForTopRequest verifies that a top-level
// request whose own Auth is Inherit picks up the collection-root auth.
func TestEffectiveAuthCollectionRootForTopRequest(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	got := s.EffectiveAuth("0/r0") // Health under the collection
	if got.Type != model.AuthBearer || got.Bearer == nil || got.Bearer.Token != "root-token" {
		t.Errorf("EffectiveAuth = %+v, want collection Bearer", got)
	}
}

// TestEffectiveAuthOwnWins verifies that a request with its own concrete
// auth short-circuits the ancestor walk and uses that auth directly.
func TestEffectiveAuthOwnWins(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	got := s.EffectiveAuth("0/f0/r1") // Delete User
	if got.Type != model.AuthAPIKey || got.APIKey == nil || got.APIKey.Name != "X-Key" {
		t.Errorf("EffectiveAuth = %+v, want own APIKey", got)
	}
}

// TestEffectiveAuthFallsToNoneWhenAllInherit verifies that a collection
// whose entire tree inherits — including the root — resolves to AuthNone
// (the load-side default for an unconfigured root).
func TestEffectiveAuthFallsToNoneWhenAllInherit(t *testing.T) {
	c := model.Collection{
		Name:     "Bare",
		Requests: []model.Request{{Name: "X", Method: model.GET, URL: "https://api/x"}},
	}
	dir := filepath.Join(t.TempDir(), "bare")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s := newSessionWith(t, dir)
	got := s.EffectiveAuth("0/r0")
	if got.Type != model.AuthNone {
		t.Errorf("EffectiveAuth = %+v, want AuthNone", got)
	}
}

// TestEffectiveAuthUnknownNodeIDIsNone verifies that calling EffectiveAuth
// with a node ID that doesn't address a real request still returns a
// usable AuthNone rather than panicking.
func TestEffectiveAuthUnknownNodeIDIsNone(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	got := s.EffectiveAuth("0/r99")
	if got.Type != model.AuthBearer {
		// Falls through to the collection's bearer auth, since the path
		// "0/r99" is still under collection 0 even though r99 doesn't exist.
		t.Errorf("EffectiveAuth(missing leaf) = %+v, want bearer fallback", got)
	}
	if junk := s.EffectiveAuth("not-a-real-id"); junk.Type != model.AuthNone {
		t.Errorf("EffectiveAuth(garbage) = %+v, want AuthNone", junk)
	}
}
