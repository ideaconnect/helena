package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
)

func TestUIStateRoundTrip(t *testing.T) {
	want := Config{
		Workspaces: []Workspace{{Name: "Default", Collections: []string{"/c/a", "/c/b"}}},
		Active:     0,
		Settings:   model.DefaultSettings(),
		UI: UIState{
			ActiveCollection: "/c/b",
			ActiveEnv:        map[string]string{"/c/a": "Local", "/c/b": "Prod"},
			OpenRequest:      &UIOpenRequest{Collection: "/c/b", NodePath: "f0/r1"},
			WindowWidth:      1200,
			WindowHeight:     800,
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
