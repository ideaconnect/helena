package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestDigestAuthRoundTrip pins #75: a request's Digest auth survives Save->Load
// and the password is externalized, never written in cleartext to the YAML.
func TestDigestAuthRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	d := &model.DigestAuth{Username: "mufasa", Password: "circle-of-life-s3cret"}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://api/", Auth: model.Auth{Type: model.AuthDigest, Digest: d}}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(dir, func(path string, de os.DirEntry, err error) error {
		if err == nil && !de.IsDir() && strings.HasSuffix(path, ".yml") {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), "circle-of-life-s3cret") {
				t.Errorf("Digest password leaked into %s", path)
			}
		}
		return nil
	})
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Requests[0].Auth
	if a.Type != model.AuthDigest || a.Digest == nil || *a.Digest != *d {
		t.Errorf("Digest auth did not round-trip: %+v", a.Digest)
	}
}
