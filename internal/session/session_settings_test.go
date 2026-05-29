package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestSettingsPersist verifies that SetSettings writes settings to disk and a
// fresh Session at the same config path reads them back.
func TestSettingsPersist(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := model.Settings{
		InsecureSkipVerify: true,
		CORSWarning:        false,
		FollowRedirects:    false,
		TimeoutSeconds:     15,
		Theme:              model.ThemeDark,
	}
	s.SetSettings(want)

	// A fresh session at the same config path must load the saved settings.
	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	if got := s2.Settings(); got != want {
		t.Errorf("Settings = %+v, want %+v", got, want)
	}
}
