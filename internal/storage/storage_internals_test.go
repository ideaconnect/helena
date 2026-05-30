package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// TestSlugFallbackForUnnameable verifies slug returns the provided
// fallback when the input has no usable characters (whitespace,
// punctuation only, or empty string). This is the path that keeps a
// request like "{{{}}}" from rendering as an empty filename.
func TestSlugFallbackForUnnameable(t *testing.T) {
	cases := map[string]string{
		"":            "fb",
		"   ":         "fb",
		"---":         "fb",
		"!!!":         "fb",
		"Hello World": "hello-world",
		" leading- ":  "leading",
	}
	for in, want := range cases {
		if got := slug(in, "fb"); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUniqueNameCollisionSuffix verifies that uniqueName escalates the
// numeric suffix on each collision so saving two requests with the
// same display name produces distinct filenames.
func TestUniqueNameCollisionSuffix(t *testing.T) {
	used := map[string]bool{}
	got := []string{
		uniqueName("a", used),
		uniqueName("a", used),
		uniqueName("a", used),
		uniqueName("b", used),
	}
	want := []string{"a", "a-2", "a-3", "b"}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("uniqueName[%d] = %q, want %q (sequence so far: %v)", i, g, want[i], got)
		}
	}
}

// TestSaveMkdirFailureSurfaces verifies that an unwriteable target
// directory (we point at a regular file's child path) surfaces the
// underlying mkdir error rather than silently failing.
func TestSaveMkdirFailureSurfaces(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Try to save under blocker/... — mkdir fails because blocker is a file.
	if err := Save(model.Collection{Name: "X"}, filepath.Join(blocker, "col")); err == nil {
		t.Error("expected error when target parent is a regular file")
	}
}

// TestLoadMissingCollectionFileErrors verifies Load returns an error
// (not the zero collection) when the directory has no collection.yml.
func TestLoadMissingCollectionFileErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Error("expected error for missing collection.yml")
	}
}

// TestLoadMalformedCollectionFileErrors verifies a corrupt
// collection.yml surfaces the YAML parse error rather than returning
// a half-constructed collection that the UI would later choke on.
func TestLoadMalformedCollectionFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte("info:\n  name: [malformed"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("expected parse error for malformed collection.yml")
	}
}

// TestLoadMalformedEnvironmentFileErrors verifies that a broken file
// under environments/ surfaces with a context-naming error rather
// than getting silently skipped.
func TestLoadMalformedEnvironmentFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	envs := filepath.Join(dir, "environments")
	if err := os.Mkdir(envs, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envs, "broken.yml"), []byte("vars: [unterminated"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.yml") {
		t.Errorf("err = %v, want one mentioning broken.yml", err)
	}
}

// TestLoadMalformedFolderFileErrors verifies that a folder with a
// corrupt folder.yml surfaces a parse error naming the folder.
func TestLoadMalformedFolderFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sub := filepath.Join(dir, "f1")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "folder.yml"), []byte("info: [bad"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "f1") {
		t.Errorf("err = %v, want mentioning f1", err)
	}
}

// TestSweepDirMissingIsNoop verifies sweepDir tolerates a missing
// directory (Save can call it on environments/ that was never
// created) by returning nil rather than failing.
func TestSweepDirMissingIsNoop(t *testing.T) {
	if err := sweepDir(filepath.Join(t.TempDir(), "nope"), nil); err != nil {
		t.Errorf("missing dir: err = %v, want nil", err)
	}
}

// TestSweepDirRemovesOrphans verifies the documented contract: any
// .yml file not in keep is removed, any folder subdirectory (one
// containing folder.yml) not in keep is removed, and unrelated
// entries are left alone.
func TestSweepDirRemovesOrphans(t *testing.T) {
	dir := t.TempDir()
	// .yml files
	for _, n := range []string{"keep.yml", "drop.yml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("info:\n  name: x\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// A "folder" subdir with folder.yml — should be removed if not kept.
	subDrop := filepath.Join(dir, "subdrop")
	if err := os.Mkdir(subDrop, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDrop, "folder.yml"), []byte("info:\n  name: x\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A random subdir that is NOT a collection folder — should be left alone.
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A random non-yml file at the top level — should be left alone.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sweepDir(dir, map[string]bool{"keep.yml": true}); err != nil {
		t.Fatalf("sweepDir: %v", err)
	}

	exists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}
	if !exists(filepath.Join(dir, "keep.yml")) {
		t.Error("keep.yml was removed")
	}
	if exists(filepath.Join(dir, "drop.yml")) {
		t.Error("drop.yml not removed")
	}
	if exists(subDrop) {
		t.Error("subdrop folder not removed")
	}
	if !exists(other) {
		t.Error("non-collection 'other' subdir removed")
	}
	if !exists(filepath.Join(dir, "notes.txt")) {
		t.Error("non-yml file removed")
	}
}

// TestReadFiles_MissingPathErrors verifies that the four read-file
// helpers surface the os.ReadFile error when the path doesn't exist
// rather than returning a zero DTO with no error.
func TestReadFiles_MissingPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yml")
	if _, err := readRequestFile(missing); err == nil {
		t.Error("readRequestFile: expected error for missing path")
	}
	if _, err := readFolderFile(missing); err == nil {
		t.Error("readFolderFile: expected error for missing path")
	}
	if _, err := readCollectionFile(missing); err == nil {
		t.Error("readCollectionFile: expected error for missing path")
	}
	if _, err := readEnvFile(missing); err == nil {
		t.Error("readEnvFile: expected error for missing path")
	}
}

// TestFileToRequestNoHTTPSection verifies that a request file without
// an http: block loads with default Method GET and an empty URL —
// matches a legitimate "scaffold" state the UI tolerates.
func TestFileToRequestNoHTTPSection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scaffold.yml"), []byte("info:\n  name: Scaffold\n  type: http\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(c.Requests))
	}
	r := c.Requests[0]
	if r.Method != "" || r.URL != "" {
		t.Errorf("scaffold = %+v, want zero method+url", r)
	}
}

// TestSaveTouchesExistingExtras verifies that Save reads the existing
// file before writing, copying its Extra map back into the new DTO so
// unknown fields (here a settings: block we hand-author) survive
// round-trips bit-for-bit. Mirrors the AGENTS invariant 1 contract.
func TestSaveTouchesExistingExtras(t *testing.T) {
	dir := t.TempDir()
	// Pre-write a collection.yml with a sibling "settings:" key the
	// model doesn't know about.
	existing := []byte(`info:
  name: ExtraColl
  type: collection
settings:
  customKey: 42
`)
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), existing, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Re-save the loaded collection (this is the round-trip path).
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "opencollection.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(out), "settings:") || !strings.Contains(string(out), "customKey") {
		t.Errorf("Extra was lost on Save round-trip:\n%s", out)
	}
}

// TestSaveEnvDirAsFileFailsCleanly verifies that an environments
// entry that already exists as a regular file (blocker) causes Save
// to error rather than panic — mkdir on environments/ refuses to
// overwrite a file.
func TestSaveEnvDirAsFileFailsCleanly(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a file at the environments path so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, "environments"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := Save(model.Collection{
		Name:         "X",
		Environments: []model.Environment{{Name: "Local"}},
	}, dir)
	if err == nil {
		t.Error("expected Save error when environments/ is a file")
	}
}

// TestLoadItemsMalformedRequestYAMLErrors verifies that a request
// file with invalid YAML surfaces a parse error naming the file
// rather than silently dropping the request.
func TestLoadItemsMalformedRequestYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"),
		[]byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yml"),
		[]byte("info: [bad"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.yml") {
		t.Errorf("err = %v, want one naming broken.yml", err)
	}
}

// TestAuthRoundTripAllTypes verifies fileToAuth handles every
// supported auth type (Basic / Bearer / API-Key / OAuth2) end-to-end
// through Save and Load. Mirrors the existing TestAuthRoundTrip in
// storage_test.go but covers the placement / grant fields the basic
// test omits.
func TestAuthRoundTripAllTypes(t *testing.T) {
	orig := model.Collection{
		Name: "A",
		Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "rt"}},
		Requests: []model.Request{
			{Name: "B", Method: model.GET, URL: "https://x/", Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "u", Password: "p"}}},
			{Name: "K", Method: model.GET, URL: "https://x/", Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "X-K", Value: "v", Placement: model.APIKeyHeader}}},
			{Name: "O", Method: model.GET, URL: "https://x/", Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{
					Grant: model.OAuth2ClientCredentials, TokenURL: "https://t/", ClientID: "c",
					ClientSecret: "s", Scope: "read", Audience: "api",
				}}},
		},
	}
	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Auth.Type != model.AuthBearer || got.Auth.Bearer.Token != "rt" {
		t.Errorf("collection auth = %+v", got.Auth)
	}
	byName := map[string]model.Request{}
	for _, r := range got.Requests {
		byName[r.Name] = r
	}
	if a := byName["B"].Auth; a.Type != model.AuthBasic || a.Basic == nil || a.Basic.Username != "u" {
		t.Errorf("Basic = %+v", a)
	}
	if a := byName["K"].Auth; a.Type != model.AuthAPIKey || a.APIKey == nil ||
		a.APIKey.Name != "X-K" || a.APIKey.Placement != model.APIKeyHeader {
		t.Errorf("APIKey = %+v", a)
	}
	if a := byName["O"].Auth; a.Type != model.AuthOAuth2 || a.OAuth2 == nil ||
		a.OAuth2.Grant != model.OAuth2ClientCredentials || a.OAuth2.ClientID != "c" {
		t.Errorf("OAuth2 = %+v", a)
	}
}

// TestSaveSweepsOrphanRequestsBetweenSaves verifies the documented
// sweep contract on save: a previous request file no longer in the
// collection's request list gets removed on the next Save.
func TestSaveSweepsOrphanRequestsBetweenSaves(t *testing.T) {
	dir := t.TempDir()
	first := model.Collection{
		Name: "A", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{
			{Name: "Keep", Method: model.GET, URL: "https://x/keep", Body: model.Body{Type: model.BodyNone}, Auth: model.Auth{Type: model.AuthInherit}},
			{Name: "Drop", Method: model.GET, URL: "https://x/drop", Body: model.Body{Type: model.BodyNone}, Auth: model.Auth{Type: model.AuthInherit}},
		},
	}
	if err := Save(first, dir); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.yml")); err != nil {
		t.Fatalf("first save didn't produce drop.yml")
	}
	// Second save drops the Drop request.
	first.Requests = first.Requests[:1]
	if err := Save(first, dir); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.yml")); err == nil {
		t.Error("drop.yml survived sweep on second Save")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.yml")); err != nil {
		t.Errorf("keep.yml removed by sweep: %v", err)
	}
}

// TestMergeKVExtrasShortCircuits verifies mergeKVExtras short-circuits
// on empty inputs (the production path for fresh writes where there
// is no prior file to merge against).
func TestMergeKVExtrasShortCircuits(t *testing.T) {
	if got := mergeKVExtras(nil, []ocKV{{Name: "a"}}); got != nil {
		t.Errorf("empty next returned non-nil: %v", got)
	}
	next := []ocKV{{Name: "a"}}
	if got := mergeKVExtras(next, nil); &got[0] != &next[0] {
		t.Error("empty prev should return next unchanged")
	}
}

// TestMergeKVExtrasPairsByName verifies that an existing header's
// Extra map gets copied into a re-saved header sharing the same
// Name. Headers absent from prev keep their original (likely nil)
// Extra.
func TestMergeKVExtrasPairsByName(t *testing.T) {
	prev := []ocKV{
		{Name: "X-Trace", Extra: map[string]yaml.Node{"prev-marker": {}}},
		{Name: "X-Empty"},
	}
	next := []ocKV{
		{Name: "X-Trace"},
		{Name: "X-New"},
	}
	got := mergeKVExtras(next, prev)
	if len(got[0].Extra) != 1 {
		t.Errorf("X-Trace Extra not merged: %+v", got[0])
	}
	if got[1].Extra != nil {
		t.Errorf("X-New picked up unexpected Extra: %+v", got[1])
	}
}

// TestMergeParamExtrasPairsByName mirrors the KV variant for the
// query-param DTO, which is a distinct type from ocKV even though
// the merge shape is identical.
func TestMergeParamExtrasPairsByName(t *testing.T) {
	prev := []ocParam{{Name: "q", Extra: map[string]yaml.Node{"prev-marker": {}}}}
	next := []ocParam{{Name: "q"}, {Name: "limit"}}
	got := mergeParamExtras(next, prev)
	if len(got[0].Extra) != 1 {
		t.Errorf("q Extra not merged: %+v", got[0])
	}
	if got[1].Extra != nil {
		t.Errorf("limit picked up Extra: %+v", got[1])
	}
	// Empty cases.
	if got := mergeParamExtras(nil, prev); got != nil {
		t.Error("empty next should return nil")
	}
	if mergeParamExtras(next, nil) == nil {
		t.Error("empty prev should return next, not nil")
	}
}

// TestMergeAuthExtrasCoversEverySubBlock verifies every auth
// sub-block (Basic, Bearer, APIKey, OAuth2) gets its Extra map
// preserved by mergeAuthExtras when both sides have the same type
// set. Also covers the nil-input guards.
func TestMergeAuthExtrasCoversEverySubBlock(t *testing.T) {
	mark := map[string]yaml.Node{"marker": {}}

	cases := []struct {
		name string
		want *ocAuth
		prev *ocAuth
	}{
		{
			name: "basic",
			want: &ocAuth{Type: "basic", Basic: &ocAuthBasic{}},
			prev: &ocAuth{Type: "basic", Basic: &ocAuthBasic{Extra: mark}},
		},
		{
			name: "bearer",
			want: &ocAuth{Type: "bearer", Bearer: &ocAuthBearer{}},
			prev: &ocAuth{Type: "bearer", Bearer: &ocAuthBearer{Extra: mark}},
		},
		{
			name: "apikey",
			want: &ocAuth{Type: "apikey", APIKey: &ocAuthAPIKey{}},
			prev: &ocAuth{Type: "apikey", APIKey: &ocAuthAPIKey{Extra: mark}},
		},
		{
			name: "oauth2",
			want: &ocAuth{Type: "oauth2", OAuth2: &ocAuthOAuth2{}},
			prev: &ocAuth{Type: "oauth2", OAuth2: &ocAuthOAuth2{Extra: mark}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mergeAuthExtras(c.want, c.prev)
			var got map[string]yaml.Node
			switch {
			case c.want.Basic != nil:
				got = c.want.Basic.Extra
			case c.want.Bearer != nil:
				got = c.want.Bearer.Extra
			case c.want.APIKey != nil:
				got = c.want.APIKey.Extra
			case c.want.OAuth2 != nil:
				got = c.want.OAuth2.Extra
			}
			if len(got) != 1 {
				t.Errorf("%s sub-block Extra not merged: got %+v", c.name, got)
			}
		})
	}

	// Nil guards.
	mergeAuthExtras(nil, &ocAuth{})
	mergeAuthExtras(&ocAuth{}, nil)
	mergeAuthExtras(nil, nil)
}

// TestSavePreservesScriptsExtraWhenHooksCleared verifies that an
// on-disk scripts: block carrying sibling keys (a tests: block from
// another tool) survives a Save where the user has cleared both
// pre+post hooks — invariant 1 says nothing externally-authored is
// lost on round-trip.
func TestSavePreservesScriptsExtraWhenHooksCleared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"),
		[]byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// scripts: { preRequest: "..."; tests: { ... } } — tests is unknown to Helena.
	if err := os.WriteFile(filepath.Join(dir, "r.yml"),
		[]byte("info:\n  name: R\n  type: http\nhttp:\n  method: GET\n  url: https://x/\nscripts:\n  tests:\n    framework: external\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// User has not authored any scripts, so re-saving would normally
	// drop the on-disk scripts block. The Extra preservation MUST
	// keep the unknown sub-keys.
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "r.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(out), "tests:") || !strings.Contains(string(out), "framework: external") {
		t.Errorf("scripts.Extra lost on round-trip:\n%s", out)
	}
}

// TestLoadEnvironmentsIgnoresSubdirsAndNonYAML verifies the filter:
// a subdirectory or a non-.yml file inside environments/ doesn't
// surface as an environment and doesn't error.
func TestLoadEnvironmentsIgnoresSubdirsAndNonYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"),
		[]byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	envs := filepath.Join(dir, "environments")
	if err := os.Mkdir(envs, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Real env file.
	if err := os.WriteFile(filepath.Join(envs, "local.yml"),
		[]byte("info:\n  name: Local\nvars:\n  - name: K\n    value: V\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Junk that must be ignored.
	if err := os.Mkdir(filepath.Join(envs, "subdir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envs, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Environments) != 1 || c.Environments[0].Name != "Local" {
		t.Errorf("envs = %+v, want only Local", c.Environments)
	}
}

// TestSaveNestedFoldersRoundTrip verifies that a request inside a
// nested folder structure (Folder A → Folder B → Request) survives
// Save → Load with the tree shape intact. Exercises saveItems
// recursion.
func TestSaveNestedFoldersRoundTrip(t *testing.T) {
	c := model.Collection{
		Name: "Nested",
		Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Outer", Auth: model.Auth{Type: model.AuthInherit},
			Folders: []model.Folder{{
				Name: "Inner", Auth: model.Auth{Type: model.AuthInherit},
				Requests: []model.Request{{
					Name: "Deep", Method: model.GET, URL: "https://x/deep",
					Body: model.Body{Type: model.BodyNone},
					Auth: model.Auth{Type: model.AuthInherit},
				}},
			}},
		}},
	}
	dir := t.TempDir()
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Folders) != 1 || got.Folders[0].Name != "Outer" {
		t.Fatalf("outer = %+v", got.Folders)
	}
	inner := got.Folders[0].Folders
	if len(inner) != 1 || inner[0].Name != "Inner" {
		t.Fatalf("inner = %+v", inner)
	}
	if len(inner[0].Requests) != 1 || inner[0].Requests[0].Name != "Deep" {
		t.Errorf("deep request = %+v", inner[0].Requests)
	}
}

// TestLoadItemsSkipsRequestsWithNonHTTPType verifies the documented
// filter: a .yml file whose info.type is something other than http
// (or empty / absent) is treated as not-a-request and skipped.
func TestLoadItemsHonorsHTTPTypeFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte("info:\n  name: X\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Type "graphql" should be skipped by loadItems.
	if err := os.WriteFile(filepath.Join(dir, "weird.yml"), []byte("info:\n  name: Weird\n  type: graphql\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Type http should be picked up.
	if err := os.WriteFile(filepath.Join(dir, "ok.yml"), []byte("info:\n  name: OK\n  type: http\nhttp:\n  method: GET\n  url: https://x/\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	names := []string{}
	for _, r := range c.Requests {
		names = append(names, r.Name)
	}
	if len(c.Requests) != 1 || c.Requests[0].Name != "OK" {
		t.Errorf("requests = %v, want only OK", names)
	}
}
