package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestWSSEAuthRoundTrip pins #79: a request's WSSE auth survives Save->Load and
// its password is externalized — never written in cleartext to the request YAML.
func TestWSSEAuthRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name: "R", Method: model.POST, URL: "https://soap/",
			Auth: model.Auth{Type: model.AuthWSSE, WSSE: &model.WSSEAuth{Username: "bob", Password: "wsse-s3cret"}},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), "wsse-s3cret") {
				t.Errorf("WSSE password leaked into %s:\n%s", path, b)
			}
		}
		return nil
	})

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Requests[0].Auth
	if a.Type != model.AuthWSSE || a.WSSE == nil || a.WSSE.Username != "bob" || a.WSSE.Password != "wsse-s3cret" {
		t.Errorf("WSSE auth did not round-trip: %+v", a)
	}
}
