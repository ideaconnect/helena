package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestLoadMissingReturnsDefault verifies that loading a non-existent path yields the default config without an error.
func TestLoadMissingReturnsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("missing config = %#v, want default", got)
	}
}

// TestLoadMissingSettingsKeepsDefaults verifies that a config file with no
// `settings:` block keeps the safe DefaultSettings (timeout 30s, follow
// redirects, CORS advisory on) instead of dropping to unsafe zero values.
func TestLoadMissingSettingsKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("workspaces:\n  - name: Default\nactive: 0\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.Settings, model.DefaultSettings()) {
		t.Errorf("Settings = %#v, want DefaultSettings %#v", got.Settings, model.DefaultSettings())
	}
}

// TestLoadPartialSettingsOverlaysDefaults verifies that a file setting only some
// settings keys overwrites just those, leaving the rest at their defaults.
func TestLoadPartialSettingsOverlaysDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	// Only theme is specified; timeout/redirects/CORS must stay default.
	if err := os.WriteFile(path, []byte("workspaces:\n  - name: Default\nsettings:\n  theme: dark\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Settings.Theme != model.ThemeDark {
		t.Errorf("Theme = %q, want dark", got.Settings.Theme)
	}
	if got.Settings.TimeoutSeconds != 30 || !got.Settings.FollowRedirects || !got.Settings.CORSWarning {
		t.Errorf("non-specified settings not defaulted: %#v", got.Settings)
	}
}

// TestSettingsMaxResponseBytesRoundTrips verifies the configurable response
// cap persists across Save/Load (#111).
func TestSettingsMaxResponseBytesRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	c := Default()
	c.Settings.MaxResponseBytes = 7 << 20
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Settings.MaxResponseBytes != 7<<20 {
		t.Errorf("MaxResponseBytes = %d, want %d", got.Settings.MaxResponseBytes, 7<<20)
	}
}

// TestLoadEmptyPathReturnsDefault verifies that Load("") short-circuits to the default config.
func TestLoadEmptyPathReturnsDefault(t *testing.T) {
	got, err := Load("")
	if err != nil || !reflect.DeepEqual(got, Default()) {
		t.Errorf("Load(\"\") = %#v, %v; want default", got, err)
	}
}

// TestSaveLoadRoundTrip verifies that a Config persists and reloads identically through Save/Load, including nested directories.
func TestSaveLoadRoundTrip(t *testing.T) {
	want := Config{
		Version: CurrentSchemaVersion,
		Workspaces: []Workspace{
			{Name: "Personal", Collections: []string{"/tmp/a", "/tmp/b"}},
			{Name: "Work"},
		},
		Active:   1,
		Settings: model.Settings{InsecureSkipVerify: true, CORSWarning: false, FollowRedirects: true, TimeoutSeconds: 15, Theme: model.ThemeDark},
	}
	path := filepath.Join(t.TempDir(), "nested", "config.yml")
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip mismatch:\n want=%#v\n got =%#v", want, got)
	}
}

// TestDefaultPathEndsWithHelenaConfig verifies DefaultPath returns a
// platform-appropriate path that ends with helena/config.yml. The
// exact prefix is OS-dependent (xdg config home, %APPDATA%, ~/Library)
// so we only assert the suffix.
func TestDefaultPathEndsWithHelenaConfig(t *testing.T) {
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	want := filepath.Join("helena", "config.yml")
	if !strings.HasSuffix(got, want) {
		t.Errorf("DefaultPath() = %q, want suffix %q", got, want)
	}
}

// TestLoadEmptyWorkspacesFallsBackToDefault verifies that a YAML file
// with no workspaces (corrupt or hand-edited) loads with the default
// single-workspace shape rather than an empty list the UI would
// stumble over.
func TestLoadEmptyWorkspacesFallsBackToDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("workspaces: []\nactive: 0\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Workspaces) != 1 || got.Workspaces[0].Name != "Default" {
		t.Errorf("Workspaces = %+v, want [{Default}]", got.Workspaces)
	}
}

// TestLoadMalformedYAMLReturnsError verifies that a structurally
// invalid YAML file surfaces the parse error to the caller rather
// than silently returning the zero Config (which would lose the
// user's real config and silently overwrite on next Save).
func TestLoadMalformedYAMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("workspaces: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected parse error from malformed YAML, got nil")
	}
}

// TestSaveMkdirFailureSurfaces verifies that a Save with an
// un-creatable parent (a path under an existing file) returns the
// mkdir error rather than silently swallowing it.
func TestSaveMkdirFailureSurfaces(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file then try to save into it as if it were a directory.
	blocker := filepath.Join(dir, "iam-a-file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err := Save(filepath.Join(blocker, "config.yml"), Default())
	if err == nil {
		t.Error("expected Save to fail when parent is a file")
	}
}

// TestLoadReadErrorSurfaces verifies that an unreadable existing file
// (e.g. a directory at the path) returns the read error rather than
// the default config — Load only falls back to default on the
// specific not-exist case.
func TestLoadReadErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	// Make the "config file" a directory so ReadFile fails with EISDIR.
	if err := os.Mkdir(filepath.Join(dir, "config.yml"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "config.yml")); err == nil {
		t.Error("expected read error when config path is a directory, got nil")
	}
}

// TestLoadClampsActive verifies that an out-of-range Active index is clamped to 0 on load.
func TestLoadClampsActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, Config{Workspaces: []Workspace{{Name: "Only"}}, Active: 9}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Active != 0 {
		t.Errorf("Active = %d, want clamped to 0", got.Active)
	}
}

// TestSaveIsAtomicOverwrite pins the stage-and-rename contract: an overwrite
// lands the new content and leaves no staging file behind. A truncate-in-place
// Save could be caught mid-write by a crash, leaving config.yml unparseable —
// and a later launch would silently fall back to an empty session.
func TestSaveIsAtomicOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := Save(path, Config{Version: CurrentSchemaVersion, Workspaces: []Workspace{{Name: "One"}}}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := Save(path, Config{Version: CurrentSchemaVersion, Workspaces: []Workspace{{Name: "Two"}}}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Workspaces) != 1 || got.Workspaces[0].Name != "Two" {
		t.Errorf("overwritten config = %+v, want the single workspace Two", got.Workspaces)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.yml" {
			t.Errorf("stray staging file left behind: %s", e.Name())
		}
	}
}

// TestSaveStagingFailureSurfaces: when the target directory exists but is not
// writable, the staged-file creation fails and Save must surface the error
// (and leave nothing behind).
func TestSaveStagingFailureSurfaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory bits are not enforced on Windows")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := Save(filepath.Join(dir, "config.yml"), Config{Version: CurrentSchemaVersion}); err == nil {
		t.Fatal("Save into a read-only directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("read-only dir should stay empty, has %d entries", len(entries))
	}
}
