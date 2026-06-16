package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// secretLadenCollection builds a collection exercising every secret field:
// collection-root Basic, a request Bearer, a folder API-key, a nested-folder
// request OAuth2, and a Secret env var — each with a unique sentinel value.
func secretLadenCollection() model.Collection {
	return model.Collection{
		Name: "Secrets",
		Auth: model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "u", Password: "PW-rootbasic"}},
		Requests: []model.Request{{
			Name: "Bearer Req", Method: model.GET, URL: "https://x/a",
			Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "TK-bearerreq"}},
		}},
		Folders: []model.Folder{{
			Name: "Folder",
			Auth: model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "X-Key", Value: "AK-folderkey"}},
			Folders: []model.Folder{{
				Name: "Nested",
				Requests: []model.Request{{
					Name: "OAuth Req", Method: model.POST, URL: "https://x/b",
					Auth: model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{ClientID: "cid", ClientSecret: "CS-oauthsecret"}},
				}},
			}},
		}},
		Environments: []model.Environment{{
			Name: "Dev",
			Variables: []model.Variable{
				{Enabled: true, Key: "PUBLIC", Value: "not-a-secret"},
				{Enabled: true, Key: "API_TOKEN", Value: "EV-envsecret", Secret: true},
			},
		}},
	}
}

var allSecretValues = []string{"PW-rootbasic", "TK-bearerreq", "AK-folderkey", "CS-oauthsecret", "EV-envsecret"}

// TestSaveExternalizesSecretsOutOfCollectionYAML is the #42 acceptance: after a
// save, no cleartext secret appears anywhere in the collection directory, and a
// fresh Load restores them all.
func TestSaveExternalizesSecretsOutOfCollectionYAML(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := filepath.Join(t.TempDir(), "col")
	if err := Save(secretLadenCollection(), dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No secret may appear in any file under the collection dir.
	for rel, content := range readTree(t, dir) {
		for _, s := range allSecretValues {
			if strings.Contains(content, s) {
				t.Errorf("collection file %q leaked secret %q", rel, s)
			}
		}
	}

	// The store file exists and (sanity) is outside the collection dir.
	path, _ := secretsPath(dir)
	if !strings.HasPrefix(path, secretsDirOverride) {
		t.Errorf("store path %q not under the override dir", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("secret store not written: %v", err)
	}

	// Everything round-trips back on Load.
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Auth.Basic.Password != "PW-rootbasic" {
		t.Errorf("root basic password = %q", got.Auth.Basic.Password)
	}
	if got.Requests[0].Auth.Bearer.Token != "TK-bearerreq" {
		t.Errorf("bearer token = %q", got.Requests[0].Auth.Bearer.Token)
	}
	if got.Folders[0].Auth.APIKey.Value != "AK-folderkey" {
		t.Errorf("folder api key = %q", got.Folders[0].Auth.APIKey.Value)
	}
	if got.Folders[0].Folders[0].Requests[0].Auth.OAuth2.ClientSecret != "CS-oauthsecret" {
		t.Errorf("oauth2 client secret = %q", got.Folders[0].Folders[0].Requests[0].Auth.OAuth2.ClientSecret)
	}
	var apiTok string
	for _, v := range got.Environments[0].Variables {
		if v.Key == "API_TOKEN" {
			apiTok = v.Value
		}
	}
	if apiTok != "EV-envsecret" {
		t.Errorf("env secret = %q", apiTok)
	}
}

// TestSplitSecretsDoesNotMutateOriginal verifies the live model keeps its
// secrets (splitSecrets works on a copy).
func TestSplitSecretsDoesNotMutateOriginal(t *testing.T) {
	c := secretLadenCollection()
	sanitized, secrets := splitSecrets(c)
	if c.Auth.Basic.Password != "PW-rootbasic" {
		t.Error("splitSecrets mutated the original collection's secret")
	}
	if sanitized.Auth.Basic.Password != "" {
		t.Error("sanitized copy still carries the secret")
	}
	if len(secrets) != 5 {
		t.Errorf("collected %d secrets, want 5: %v", len(secrets), secrets)
	}
}

// TestLoadMigratesOldPlaintextCollection verifies a pre-#42 collection whose
// YAML still holds a cleartext secret loads unchanged (no store), then migrates
// the secret out of the YAML on its next save.
func TestLoadMigratesOldPlaintextCollection(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	root := "info:\n  name: Old\n  type: collection\n"
	if err := os.WriteFile(filepath.Join(dir, collectionFile), []byte(root), 0o644); err != nil {
		t.Fatal(err)
	}
	req := "info:\n  name: R\n  type: http\n  seq: 1\nhttp:\n  method: GET\n  url: https://x/p\n  auth:\n    type: bearer\n    bearer:\n      token: OLD-PLAINTEXT\n"
	if err := os.WriteFile(filepath.Join(dir, "r.yml"), []byte(req), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load old: %v", err)
	}
	if c.Requests[0].Auth.Bearer.Token != "OLD-PLAINTEXT" {
		t.Fatalf("old plaintext token not read: %q", c.Requests[0].Auth.Bearer.Token)
	}

	if err := Save(c, dir); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	for rel, content := range readTree(t, dir) {
		if strings.Contains(content, "OLD-PLAINTEXT") {
			t.Errorf("migration left the secret in %q", rel)
		}
	}
	got, _ := Load(dir)
	if got.Requests[0].Auth.Bearer.Token != "OLD-PLAINTEXT" {
		t.Errorf("token lost after migration: %q", got.Requests[0].Auth.Bearer.Token)
	}
}

// TestRemovingSecretSweepsStore verifies clearing the last secret deletes the
// store file.
func TestRemovingSecretSweepsStore(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := filepath.Join(t.TempDir(), "col")
	c := model.Collection{
		Name: "C",
		Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "T"}},
	}
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, _ := secretsPath(dir)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("store not created: %v", err)
	}

	c.Auth.Bearer.Token = ""
	if err := Save(c, dir); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("store not swept after clearing the secret (err=%v)", err)
	}
}

// TestApplySecretsKeepsExistingValues verifies applySecrets never overwrites a
// field that already has a value, and is a no-op for an empty store.
func TestApplySecretsKeepsExistingValues(t *testing.T) {
	c := model.Collection{Auth: model.Auth{Bearer: &model.BearerAuth{Token: "live"}}}
	applySecrets(&c, map[string]string{"col/auth/bearer.token": "stored"})
	if c.Auth.Bearer.Token != "live" {
		t.Errorf("applySecrets overwrote a non-empty field: %q", c.Auth.Bearer.Token)
	}
	c.Auth.Bearer.Token = ""
	applySecrets(&c, nil) // empty store: no-op
	if c.Auth.Bearer.Token != "" {
		t.Error("applySecrets(nil) changed a field")
	}
}
