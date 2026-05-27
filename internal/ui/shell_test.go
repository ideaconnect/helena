package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

func TestNewMainUIBuilds(t *testing.T) {
	test.NewApp()

	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)

	if m.Root() == nil {
		t.Fatal("Root() is nil")
	}
	if m.Method.Selected != "GET" {
		t.Errorf("default method = %q, want GET", m.Method.Selected)
	}
	if m.Workspace.Selected != "Default" {
		t.Errorf("workspace = %q, want Default", m.Workspace.Selected)
	}
	if len(m.Request.Items) != 3 || len(m.Response.Items) != 3 {
		t.Errorf("tabs = %d/%d, want 3/3", len(m.Request.Items), len(m.Response.Items))
	}

	// Lay it out in a headless window to catch construction/layout panics.
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(1100, 720))
	w.Close()
}
