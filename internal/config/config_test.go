package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
)

func TestLoadMissingReturnsDefault(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("missing config = %#v, want default", got)
	}
}

func TestLoadEmptyPathReturnsDefault(t *testing.T) {
	got, err := Load("")
	if err != nil || !reflect.DeepEqual(got, Default()) {
		t.Errorf("Load(\"\") = %#v, %v; want default", got, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	want := Config{
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
