package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAWSV4AuthRoundTrip pins #76: a request's AWS SigV4 auth survives
// Save->Load and both secrets (secret access key + session token) are
// externalized, never written in cleartext to the request YAML.
func TestAWSV4AuthRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	v := &model.AWSV4Auth{
		AccessKeyID:     "AKID",
		SecretAccessKey: "super-s3cret-key",
		Region:          "eu-west-1",
		Service:         "s3",
		SessionToken:    "sess-s3cret-token",
	}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://api/", Auth: model.Auth{Type: model.AuthAWSV4, AWSV4: v}}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"super-s3cret-key", "sess-s3cret-token"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
				if b, _ := os.ReadFile(path); strings.Contains(string(b), secret) {
					t.Errorf("AWSV4 secret %q leaked into %s", secret, path)
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
	if a.Type != model.AuthAWSV4 || a.AWSV4 == nil || *a.AWSV4 != *v {
		t.Errorf("AWSV4 auth did not round-trip: %+v", a.AWSV4)
	}
}
