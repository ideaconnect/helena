package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFyneAppTOMLIDMatchesConst guards against drift between the app ID baked
// into the binary (appID) and the ID in FyneApp.toml that the packaging tools
// read — a mismatch would ship bundles whose identity differs from the running
// app (#34).
func TestFyneAppTOMLIDMatchesConst(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "FyneApp.toml"))
	if err != nil {
		t.Fatalf("read FyneApp.toml: %v", err)
	}
	want := `ID = "` + appID + `"`
	if !strings.Contains(string(data), want) {
		t.Errorf("FyneApp.toml does not contain %q; ID drifted from the appID const", want)
	}
}

// TestVersionString covers the --version output formatting (#26): the default
// dev build, a tagged release with a long commit (trimmed), and a full build
// with a date.
func TestVersionString(t *testing.T) {
	cases := []struct {
		name                    string
		version, commit, date   string
		wantContains, wantOmits []string
	}{
		{
			name:         "dev default",
			version:      "dev",
			wantContains: []string{"helena dev"},
		},
		{
			name:         "tagged with long commit",
			version:      "v0.1.0",
			commit:       "0123456789abcdef0123",
			wantContains: []string{"helena v0.1.0", "0123456789ab"},
			wantOmits:    []string{"0123456789abcdef"}, // commit trimmed to 12
		},
		{
			name:         "with date",
			version:      "v0.2.0",
			commit:       "abc123",
			date:         "2026-06-16",
			wantContains: []string{"helena v0.2.0", "(abc123)", "built 2026-06-16"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := versionString(c.version, c.commit, c.date)
			for _, w := range c.wantContains {
				if !strings.Contains(got, w) {
					t.Errorf("versionString = %q, want it to contain %q", got, w)
				}
			}
			for _, w := range c.wantOmits {
				if strings.Contains(got, w) {
					t.Errorf("versionString = %q, want it to omit %q", got, w)
				}
			}
		})
	}
}

// TestWindowTitle verifies the window title carries the version only for a
// non-dev build.
func TestWindowTitle(t *testing.T) {
	if got := windowTitle("dev"); got != "Helena" {
		t.Errorf("dev title = %q, want Helena", got)
	}
	if got := windowTitle(""); got != "Helena" {
		t.Errorf("empty title = %q, want Helena", got)
	}
	if got := windowTitle("v1.0.0"); got != "Helena v1.0.0" {
		t.Errorf("release title = %q, want Helena v1.0.0", got)
	}
}
