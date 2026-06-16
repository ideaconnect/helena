package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestEmptyStateShownWithNoCollections verifies the first-run starter panel is
// visible when no collection is loaded and hides once one exists (#58).
func TestEmptyStateShownWithNoCollections(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	if !m.emptyState.Visible() {
		t.Fatal("empty-state panel not shown with zero collections")
	}

	// Open a collection, then refresh — the panel must hide.
	c := model.Collection{Name: "Demo", Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://x"}}}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatal(err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	m.refreshEmptyState()
	if m.emptyState.Visible() {
		t.Error("empty-state panel still shown after a collection was opened")
	}
}

// TestLoadSampleOpensCollection verifies the Load-sample action materializes
// and opens the bundled sample, hiding the empty state (#57/#58).
func TestLoadSampleOpensCollection(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	if len(sess.Collections()) != 0 {
		t.Fatalf("fresh session has %d collections", len(sess.Collections()))
	}

	m.loadSampleFrom(t.TempDir())

	if len(sess.Collections()) != 1 {
		t.Fatalf("after loadSample: %d collections, want 1", len(sess.Collections()))
	}
	if m.emptyState.Visible() {
		t.Error("empty-state still shown after loading the sample")
	}
}
