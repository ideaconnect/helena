package assets

import "testing"

// TestIconReturnsResource verifies Icon loads an embedded SVG and
// returns a non-nil Fyne resource named after the icon. Covers the
// happy path of the lookup.
func TestIconReturnsResource(t *testing.T) {
	r := Icon("paper-plane")
	if r == nil {
		t.Fatal("Icon returned nil for paper-plane")
	}
	if r.Name() != "paper-plane.svg" {
		t.Errorf("name = %q, want paper-plane.svg", r.Name())
	}
	if len(r.Content()) == 0 {
		t.Error("Icon returned empty content")
	}
}

// TestIconMissingPanics verifies the documented panic-on-missing
// contract — a typo in a static call site fails fast at first
// access rather than silently returning a zero resource.
func TestIconMissingPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Icon(missing) did not panic")
		}
	}()
	_ = Icon("definitely-not-an-icon")
}

// TestFontReturnsResource verifies Font loads an embedded TTF and returns a
// non-nil Fyne resource named after the font (the happy path).
func TestFontReturnsResource(t *testing.T) {
	r := Font("Inter-Regular")
	if r == nil {
		t.Fatal("Font returned nil for Inter-Regular")
	}
	if r.Name() != "Inter-Regular.ttf" {
		t.Errorf("name = %q, want Inter-Regular.ttf", r.Name())
	}
	if len(r.Content()) == 0 {
		t.Error("Font returned empty content")
	}
}

// TestFontMissingPanics verifies the documented panic-on-missing contract for
// fonts, mirroring Icon.
func TestFontMissingPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Font(missing) did not panic")
		}
	}()
	_ = Font("definitely-not-a-font")
}
