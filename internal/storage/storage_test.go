package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// TestSaveLeavesNoTempFiles verifies the atomic-write path cleans up after
// itself: a successful Save renames its temp file over the target, leaving no
// stray .helena-*.tmp behind.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Save(sampleCollection(), dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	walkNoTemp := func(d string) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".helena-") {
				t.Errorf("leftover temp file in %s: %s", d, e.Name())
			}
		}
	}
	walkNoTemp(dir)
	// Folders are written into subdirectories too.
	subs, _ := os.ReadDir(dir)
	for _, s := range subs {
		if s.IsDir() {
			walkNoTemp(filepath.Join(dir, s.Name()))
		}
	}
}

// sampleCollection returns a small in-memory collection used as the fixture
// for the round-trip tests. Auth fields use the same defaults that fileToAuth
// applies on load (Inherit for requests/folders, None for the collection
// root) so the round-trip comparison stays clean.
func sampleCollection() model.Collection {
	return model.Collection{
		ID:   model.NewID(),
		Name: "Demo API",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			ID:     model.NewID(),
			Name:   "Health",
			Method: model.GET,
			URL:    "https://{{base}}/health",
			Headers: []model.KeyValue{
				{Enabled: true, Key: "Accept", Value: "application/json"},
				{Enabled: false, Key: "X-Debug", Value: "1"},
			},
			Params: []model.KeyValue{{Enabled: true, Key: "verbose", Value: "true"}},
			Body:   model.Body{Type: model.BodyNone},
			Auth:   model.Auth{Type: model.AuthInherit},
		}},
		Folders: []model.Folder{{
			ID:   model.NewID(),
			Name: "Users",
			Auth: model.Auth{Type: model.AuthInherit},
			Requests: []model.Request{{
				ID:     model.NewID(),
				Name:   "Create User",
				Method: model.POST,
				URL:    "https://{{base}}/users",
				Body:   model.Body{Type: model.BodyJSON, Content: "{\n  \"name\": \"Ada\"\n}"},
				Auth:   model.Auth{Type: model.AuthInherit},
			}},
		}},
		Environments: []model.Environment{{
			ID:        model.NewID(),
			Name:      "Local",
			Variables: []model.Variable{{Enabled: true, Key: "base", Value: "http://localhost:8080"}},
		}},
	}
}

// TestSaveLoadRoundTrip verifies that a collection saved to disk and then
// reloaded equals the original (ignoring freshly-assigned IDs).
func TestSaveLoadRoundTrip(t *testing.T) {
	orig := sampleCollection()

	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	clearIDs(&orig)
	clearIDs(&got)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n orig=%#v\n got =%#v", orig, got)
	}
}

// TestRequestFileUsesSpecKeys verifies that a marshalled request file uses the
// OpenCollection YAML key names (info/http/headers/body, type:http, disabled).
func TestRequestFileUsesSpecKeys(t *testing.T) {
	rf := requestToFile(model.Request{
		Name:    "Create User",
		Method:  model.POST,
		URL:     "https://api/users",
		Headers: []model.KeyValue{{Enabled: false, Key: "X-Debug", Value: "1"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}, 1)

	data, err := yaml.Marshal(rf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)
	for _, want := range []string{
		"info:", "name: Create User", "type: http",
		"http:", "method: POST", "url: https://api/users",
		"headers:", "name: X-Debug", "disabled: true",
		"body:", "type: json", "data:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("marshalled request missing %q:\n%s", want, out)
		}
	}
}

// clearIDs zeroes the random IDs on a collection tree so two collections that
// only differ by freshly-generated IDs compare equal.
func clearIDs(c *model.Collection) {
	c.ID = ""
	for i := range c.Requests {
		c.Requests[i].ID = ""
	}
	for i := range c.Folders {
		clearFolderIDs(&c.Folders[i])
	}
	for i := range c.Environments {
		c.Environments[i].ID = ""
	}
}

// clearFolderIDs recursively zeroes IDs on a folder and its descendants.
func clearFolderIDs(f *model.Folder) {
	f.ID = ""
	for i := range f.Requests {
		f.Requests[i].ID = ""
	}
	for i := range f.Folders {
		clearFolderIDs(&f.Folders[i])
	}
}

// TestAuthRoundTrip verifies that every supported auth type — collection-,
// folder-, and request-level — survives Save → Load with the same shape it
// started with, and that the on-disk YAML carries an `auth:` block under
// the expected parents.
func TestAuthRoundTrip(t *testing.T) {
	orig := model.Collection{
		Name: "Auth sample",
		Auth: model.Auth{
			Type:  model.AuthBasic,
			Basic: &model.BasicAuth{Username: "root", Password: "{{PASS}}"},
		},
		Folders: []model.Folder{{
			Name: "Secure",
			Auth: model.Auth{
				Type:   model.AuthBearer,
				Bearer: &model.BearerAuth{Token: "{{TOKEN}}"},
			},
			Requests: []model.Request{{
				Name:   "Keyed",
				Method: model.GET,
				URL:    "https://api/v1",
				Auth: model.Auth{
					Type: model.AuthAPIKey,
					APIKey: &model.APIKeyAuth{
						Name:      "X-API-Key",
						Value:     "{{KEY}}",
						Placement: model.APIKeyHeader,
					},
				},
			}},
		}},
		Requests: []model.Request{{
			Name:   "OAuth call",
			Method: model.POST,
			URL:    "https://api/oauth",
			Auth: model.Auth{
				Type: model.AuthOAuth2,
				OAuth2: &model.OAuth2Auth{
					Grant:        model.OAuth2ClientCredentials,
					TokenURL:     "https://auth/token",
					ClientID:     "id",
					ClientSecret: "secret",
					Scope:        "read",
				},
			},
		}},
	}

	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Auth.Type != model.AuthBasic || got.Auth.Basic == nil ||
		got.Auth.Basic.Username != "root" || got.Auth.Basic.Password != "{{PASS}}" {
		t.Errorf("collection auth = %+v", got.Auth)
	}
	if len(got.Folders) != 1 {
		t.Fatalf("folders = %d", len(got.Folders))
	}
	fa := got.Folders[0].Auth
	if fa.Type != model.AuthBearer || fa.Bearer == nil || fa.Bearer.Token != "{{TOKEN}}" {
		t.Errorf("folder auth = %+v", fa)
	}
	if len(got.Folders[0].Requests) != 1 {
		t.Fatalf("folder requests = %d", len(got.Folders[0].Requests))
	}
	ra := got.Folders[0].Requests[0].Auth
	if ra.Type != model.AuthAPIKey || ra.APIKey == nil ||
		ra.APIKey.Name != "X-API-Key" || ra.APIKey.Value != "{{KEY}}" ||
		ra.APIKey.Placement != model.APIKeyHeader {
		t.Errorf("request auth = %+v", ra)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("collection requests = %d", len(got.Requests))
	}
	oa := got.Requests[0].Auth
	if oa.Type != model.AuthOAuth2 || oa.OAuth2 == nil ||
		oa.OAuth2.Grant != model.OAuth2ClientCredentials ||
		oa.OAuth2.TokenURL != "https://auth/token" ||
		oa.OAuth2.ClientID != "id" || oa.OAuth2.ClientSecret != "secret" {
		t.Errorf("oauth2 auth = %+v", oa)
	}

	// Confirm the on-disk YAML carries `auth:` keys.
	collectionData, err := os.ReadFile(filepath.Join(dir, collectionFile))
	if err != nil {
		t.Fatalf("read collection yaml: %v", err)
	}
	if !strings.Contains(string(collectionData), "auth:") {
		t.Errorf("collection yaml missing auth key:\n%s", collectionData)
	}
}

// TestAuthInheritDefaultsOmittedFromYAML verifies that the default Inherit
// auth on a fresh request does not write an auth block to disk; the YAML
// stays clean for collections that never configured auth.
func TestAuthInheritDefaultsOmittedFromYAML(t *testing.T) {
	c := model.Collection{
		Name: "Clean",
		Requests: []model.Request{{
			Name:   "Plain",
			Method: model.GET,
			URL:    "https://api/x",
			Auth:   model.Auth{Type: model.AuthInherit},
		}},
	}
	dir := t.TempDir()
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ymlExt) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(b), "auth:") {
			t.Errorf("%s should not carry an auth block when Inherit:\n%s", e.Name(), b)
		}
	}
}

// TestDocsRoundTrip verifies that the per-request docs field survives a save
// and load, and that it is serialized as a first-class `docs:` key (not via
// the Extra catch-all).
func TestDocsRoundTrip(t *testing.T) {
	docs := "# Example\n\nLists `verbose` users.\n\n- one\n- two\n\n```json\n{\"ok\":true}\n```\n"
	orig := model.Collection{
		Name: "Docs sample",
		Requests: []model.Request{{
			Name:   "Documented",
			Method: model.GET,
			URL:    "https://example.com/docs",
			Docs:   docs,
		}},
	}

	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(got.Requests))
	}
	if got.Requests[0].Docs != docs {
		t.Errorf("docs mismatch\nwant: %q\n got: %q", docs, got.Requests[0].Docs)
	}

	// Confirm the on-disk YAML uses the docs key (so it is a first-class field,
	// not just preserved via Extra).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var yamlPath string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") && e.Name() != collectionFile {
			yamlPath = filepath.Join(dir, e.Name())
			break
		}
	}
	if yamlPath == "" {
		t.Fatal("request yaml not found on disk")
	}
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "docs: ") && !strings.Contains(string(data), "docs:|") {
		t.Errorf("request yaml missing docs key:\n%s", data)
	}
}
