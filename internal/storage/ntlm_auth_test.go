package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestNTLMAuthRoundTrip pins #78: a request's NTLM auth survives Save->Load and
// the password is externalized, never written in cleartext to the YAML.
func TestNTLMAuthRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	n := &model.NTLMAuth{Username: "alice", Password: "ntlm-s3cret", Domain: "CORP", Workstation: "WS1"}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://api/", Auth: model.Auth{Type: model.AuthNTLM, NTLM: n}}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), "ntlm-s3cret") {
				t.Errorf("NTLM password leaked into %s", path)
			}
		}
		return nil
	})
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Requests[0].Auth
	if a.Type != model.AuthNTLM || a.NTLM == nil || *a.NTLM != *n {
		t.Errorf("NTLM auth did not round-trip: %+v", a.NTLM)
	}
}
