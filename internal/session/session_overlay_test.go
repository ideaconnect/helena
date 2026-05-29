package session

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestEnvOverlaySetGet verifies the basic round-trip and the empty-name
// no-op guard.
func TestEnvOverlaySetGet(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.SetEnvOverlay("TOKEN", "abc")
	if v, ok := s.EnvOverlay("TOKEN"); !ok || v != "abc" {
		t.Errorf("EnvOverlay TOKEN = %q ok=%v, want abc true", v, ok)
	}
	if _, ok := s.EnvOverlay("MISSING"); ok {
		t.Errorf("EnvOverlay MISSING ok=true, want false")
	}
	s.SetEnvOverlay("", "noise")
	if _, ok := s.EnvOverlay(""); ok {
		t.Errorf("empty name should be ignored")
	}
}

// TestEnvOverlayClear verifies ClearEnvOverlay drops every entry.
func TestEnvOverlayClear(t *testing.T) {
	s, _ := New("")
	s.SetEnvOverlay("A", "1")
	s.SetEnvOverlay("B", "2")
	s.ClearEnvOverlay()
	if _, ok := s.EnvOverlay("A"); ok {
		t.Errorf("A still set after Clear")
	}
	if _, ok := s.EnvOverlay("B"); ok {
		t.Errorf("B still set after Clear")
	}
}

// TestResolverOverlayLayering verifies that an overlay entry overrides
// the active environment value of the same name and that overlay-only
// names resolve too.
func TestResolverOverlayLayering(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Environments: []model.Environment{{
			Name: "Local",
			Variables: []model.Variable{
				{Enabled: true, Key: "BASE", Value: "underlying"},
			},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	s.SetActiveEnv("Local")

	// Underlying env wins when overlay is empty.
	if v, _ := s.Resolver().Lookup("BASE"); v != "underlying" {
		t.Errorf("BASE = %q, want underlying", v)
	}

	// Overlay overrides.
	s.SetEnvOverlay("BASE", "overridden")
	if v, _ := s.Resolver().Lookup("BASE"); v != "overridden" {
		t.Errorf("BASE after overlay = %q, want overridden", v)
	}

	// Overlay-only key resolves.
	s.SetEnvOverlay("ONLY_OVERLAY", "value")
	if v, _ := s.Resolver().Lookup("ONLY_OVERLAY"); v != "value" {
		t.Errorf("ONLY_OVERLAY = %q, want value", v)
	}
}

// TestEnvOverlayNotPersisted verifies SaveActiveCollection does NOT
// write overlay entries into the on-disk environment file — invariant 9
// from AGENTS.md.
func TestEnvOverlayNotPersisted(t *testing.T) {
	c := model.Collection{
		Name: "Demo",
		Auth: model.Auth{Type: model.AuthNone},
		Environments: []model.Environment{{
			Name: "Local",
			Variables: []model.Variable{
				{Enabled: true, Key: "BASE", Value: "underlying"},
			},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "cfg.yml"))
	_ = s.OpenCollection(dir)
	s.SetActiveEnv("Local")

	s.SetEnvOverlay("SCRIPT_SET", "secret")
	if err := s.SaveActiveCollection(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, v := range reloaded.Environments[0].Variables {
		if v.Key == "SCRIPT_SET" {
			t.Errorf("overlay leaked to disk as environment variable: %+v", v)
		}
	}
}

// TestEnvOverlayConcurrentSafe exercises the RWMutex by hammering
// SetEnvOverlay and Resolver().Lookup from multiple goroutines — we
// don't assert specific values, just that the race detector stays quiet.
func TestEnvOverlayConcurrentSafe(t *testing.T) {
	s, _ := New("")
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.SetEnvOverlay("K", "v")
			_ = i
		}(i)
		go func() {
			defer wg.Done()
			_, _ = s.Resolver().Lookup("K")
		}()
	}
	wg.Wait()
}
