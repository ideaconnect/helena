package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestEditSettingsBuildsWithTLSCaption verifies the Settings dialog (which now
// includes the insecure-TLS risk caption, #45) constructs and shows without
// panicking against a headless window.
func TestEditSettingsBuildsWithTLSCaption(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(900, 700))
	defer w.Close()
	m.SetWindow(w)

	// Must not panic; the dialog is non-blocking (Show returns immediately).
	m.editSettings()
}
