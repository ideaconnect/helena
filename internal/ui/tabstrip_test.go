package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// TestRequestTabLabelAndActive verifies setTab fills the method chip + name and
// setActive toggles the highlight rectangle.
func TestRequestTabLabelAndActive(t *testing.T) {
	test.NewApp()
	rt := newRequestTab(func() {}, func() {})
	rt.setTab("POST", "Create User")
	if rt.method.Text != "POST" || rt.method.Color != methodColor("POST") {
		t.Errorf("method chip = %q %v, want POST tinted", rt.method.Text, rt.method.Color)
	}
	if rt.name.Text != "Create User" {
		t.Errorf("name = %q, want Create User", rt.name.Text)
	}
	// A blank name renders as "Untitled" (a fresh scratch tab).
	rt.setTab("GET", "")
	if rt.name.Text != "Untitled" {
		t.Errorf("blank name = %q, want Untitled", rt.name.Text)
	}

	rt.setActive(true)
	if rt.bg.FillColor != theme.Color(theme.ColorNameSelection) {
		t.Error("setActive(true) did not fill the highlight rectangle")
	}
	rt.setActive(false)
	if rt.bg.FillColor != color.Transparent {
		t.Error("setActive(false) did not clear the highlight rectangle")
	}
}

// TestRequestTabSelectAndClose verifies tapping the body selects and the close
// button fires the close callback.
func TestRequestTabSelectAndClose(t *testing.T) {
	test.NewApp()
	selected, closed := 0, 0
	rt := newRequestTab(func() { selected++ }, func() { closed++ })

	rt.Tapped(&fyne.PointEvent{})
	if selected != 1 {
		t.Errorf("Tapped fired onSelect %d times, want 1", selected)
	}
	rt.closeBtn.OnTapped()
	if closed != 1 {
		t.Errorf("close button fired onClose %d times, want 1", closed)
	}
}
