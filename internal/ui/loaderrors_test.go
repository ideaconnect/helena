package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestLoadErrorReportEmptyWhenClean verifies a session with no load failures
// produces no report and SurfaceLoadErrors is a safe no-op (#108).
func TestLoadErrorReportEmptyWhenClean(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	if got := m.loadErrorReport(); got != "" {
		t.Errorf("clean session report = %q, want empty", got)
	}
	m.SurfaceLoadErrors() // no window, must not panic
}

// TestLoadErrorReportListsFailedCollection verifies the report names the
// collection directory that failed to load.
func TestLoadErrorReportListsFailedCollection(t *testing.T) {
	test.NewApp()
	colDir := filepath.Join(t.TempDir(), "broken-api")
	if err := storage.Save(model.Collection{Name: "Broken"}, colDir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	s, _ := session.New(cfgPath)
	if err := s.OpenCollection(colDir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	if err := os.RemoveAll(colDir); err != nil {
		t.Fatal(err)
	}
	s2, _ := session.New(cfgPath)

	m := NewMainUI(s2)
	report := m.loadErrorReport()
	if !strings.Contains(report, colDir) {
		t.Errorf("report %q does not name the failed collection %q", report, colDir)
	}
}
