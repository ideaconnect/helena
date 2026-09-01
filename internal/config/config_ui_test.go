package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestUIStateRoundTrip verifies that UIState (active collection, env map, open
// request, open tabs + active tab, window size, response wrap) survives Save/Load.
func TestUIStateRoundTrip(t *testing.T) {
	want := Config{
		Version:    CurrentSchemaVersion,
		Workspaces: []Workspace{{Name: "Default", Collections: []string{"/c/a", "/c/b"}}},
		Active:     0,
		Settings:   model.DefaultSettings(),
		UI: UIState{
			ActiveCollection: "/c/b",
			ActiveEnv:        map[string]string{"/c/a": "Local", "/c/b": "Prod"},
			OpenRequest:      &UIOpenRequest{Collection: "/c/b", NodePath: "f0/r1"},
			OpenTabs: []UIOpenTab{
				{Collection: "/c/a", RequestID: "id-a"},
				{Collection: "/c/b", RequestID: "id-b"},
			},
			ActiveTab:    1,
			WindowWidth:  1200,
			WindowHeight: 800,
			ResponseWrap: true,
		},
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("UIState round-trip mismatch:\n want=%#v\n got =%#v", want, got)
	}
}

// TestUIStateOmitsEmptyTabs verifies that a config without open tabs writes no
// openTabs / activeTab keys, so users who never open multiple tabs keep a clean
// config.yml.
func TestUIStateOmitsEmptyTabs(t *testing.T) {
	c := Default()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "openTabs") || strings.Contains(string(data), "activeTab") {
		t.Errorf("empty config wrote tab keys:\n%s", data)
	}
}

// TestUIStateOmitsDefaultResponseWrap verifies wrap-off (the default) writes no
// responseWrap key, so the persisted toggle only appears once a user turns it on.
func TestUIStateOmitsDefaultResponseWrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "responseWrap") {
		t.Errorf("wrap-off config wrote a responseWrap key:\n%s", data)
	}

	// Turning it on must write the key (the omitempty must not swallow a real
	// preference), and it must load back as true.
	c := Default()
	c.UI.ResponseWrap = true
	if err := Save(path, c); err != nil {
		t.Fatalf("Save (wrap on): %v", err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "responseWrap: true") {
		t.Errorf("wrap-on config did not write responseWrap:\n%s", data)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.UI.ResponseWrap {
		t.Error("ResponseWrap did not survive Save/Load")
	}
}
