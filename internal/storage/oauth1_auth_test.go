package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestOAuth1AuthRoundTrip pins #77: a request's OAuth1 auth survives Save->Load
// and both secrets (consumer + token) are externalized, never written in
// cleartext to the request YAML.
func TestOAuth1AuthRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	o := &model.OAuth1Auth{ConsumerKey: "ck", ConsumerSecret: "consumer-s3cret", Token: "tok", TokenSecret: "token-s3cret"}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://api/", Auth: model.Auth{Type: model.AuthOAuth1, OAuth1: o}}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"consumer-s3cret", "token-s3cret"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
				if b, _ := os.ReadFile(path); strings.Contains(string(b), secret) {
					t.Errorf("OAuth1 secret %q leaked into %s", secret, path)
				}
			}
			return nil
		})
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Requests[0].Auth
	if a.Type != model.AuthOAuth1 || a.OAuth1 == nil || *a.OAuth1 != *o {
		t.Errorf("OAuth1 auth did not round-trip: %+v", a.OAuth1)
	}
}
