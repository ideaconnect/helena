package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMigratesLegacyConfig verifies a pre-versioning (no `version:` key)
// config is recognised as v0, migrated forward, and stamped to the current
// schema version. The v0→v1 step also normalizes the old unsafe
// TimeoutSeconds=0 to the default (#50).
func TestLoadMigratesLegacyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	// No `version:` key, and an explicit timeout of 0 (the legacy unlimited).
	body := "workspaces:\n  - name: Default\nactive: 0\nsettings:\n  timeoutSeconds: 0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != CurrentSchemaVersion {
		t.Errorf("Version = %d, want %d (migrated)", got.Version, CurrentSchemaVersion)
	}
	if got.Settings.TimeoutSeconds != 30 {
		t.Errorf("legacy TimeoutSeconds=0 not normalized: got %d, want 30", got.Settings.TimeoutSeconds)
	}
}

// TestLoadFutureVersionWarnsWithoutDataLoss verifies a config from a newer
// build is loaded best-effort (known fields preserved, version kept) and a
// warning is surfaced rather than the file being silently downgraded (#50).
func TestLoadFutureVersionWarnsWithoutDataLoss(t *testing.T) {
	var warned string
	orig := warnf
	warnf = func(format string, args ...any) { warned = format }
	defer func() { warnf = orig }()

	path := filepath.Join(t.TempDir(), "config.yml")
	body := "version: 999\nworkspaces:\n  - name: Future\nactive: 0\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != 999 {
		t.Errorf("future Version not preserved: got %d, want 999", got.Version)
	}
	if len(got.Workspaces) != 1 || got.Workspaces[0].Name != "Future" {
		t.Errorf("future config lost known fields: %+v", got.Workspaces)
	}
	if !strings.Contains(warned, "schema version") {
		t.Errorf("no future-version warning emitted; got %q", warned)
	}
}

// TestMigrateNoOpAtCurrentVersion verifies a config already at the current
// version passes through migrate unchanged.
func TestMigrateNoOpAtCurrentVersion(t *testing.T) {
	c := Default()
	c.Settings.TimeoutSeconds = 5 // a valid non-default value must survive
	got := migrate(c)
	if got.Version != CurrentSchemaVersion || got.Settings.TimeoutSeconds != 5 {
		t.Errorf("migrate mutated a current-version config: %+v", got.Settings)
	}
}
