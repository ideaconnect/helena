package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestShowSaving verifies the close spinner is a no-op without a window and adds
// a modal overlay once a window is set.
func TestShowSaving(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	// No window yet: must not panic and must not add anything.
	m.ShowSaving()

	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	before := len(w.Canvas().Overlays().List())
	m.ShowSaving()
	if after := len(w.Canvas().Overlays().List()); after <= before {
		t.Fatalf("ShowSaving added no modal overlay (before=%d after=%d)", before, after)
	}
}
