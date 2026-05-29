package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestFindRequestByPath verifies the slash-separated name path resolves
// to the right request, including nested folders. Unknown paths return
// false. Empty paths and leading slashes are tolerated.
func TestFindRequestByPath(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth", Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{{Name: "Login", Method: model.POST, URL: "https://api/login"}},
			Folders: []model.Folder{{
				Name: "OAuth", Auth: model.Auth{Type: model.AuthInherit},
				Requests: []model.Request{{Name: "Token", Method: model.POST, URL: "https://api/oauth/token"}},
			}},
		}},
		Requests: []model.Request{
			{Name: "Health", Method: model.GET, URL: "https://api/health"},
		},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	cases := []struct {
		path string
		want string // expected r.Name; "" → not found
	}{
		{"Health", "Health"},
		{"Auth/Login", "Login"},
		{"/Auth/Login", "Login"},      // leading slash tolerated
		{"Auth/OAuth/Token", "Token"}, // nested folder
		{"Missing", ""},
		{"Auth/Missing", ""},
		{"Auth/OAuth/Missing", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got, ok := s.FindRequestByPath(tc.path)
		if tc.want == "" {
			if ok {
				t.Errorf("%q: ok=true, want false (got %s)", tc.path, got.Name)
			}
			continue
		}
		if !ok {
			t.Errorf("%q: ok=false, want %s", tc.path, tc.want)
			continue
		}
		if got.Name != tc.want {
			t.Errorf("%q: got %s, want %s", tc.path, got.Name, tc.want)
		}
	}
}

// TestFindRequestByPathNoActiveCollection verifies the lookup safely
// returns false when no collection is loaded.
func TestFindRequestByPathNoActiveCollection(t *testing.T) {
	s, _ := New("")
	if _, ok := s.FindRequestByPath("anything"); ok {
		t.Error("expected false when no active collection")
	}
}
