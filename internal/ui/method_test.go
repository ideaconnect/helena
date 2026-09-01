package ui

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/idct/helena/internal/model"
)

// TestQueryMethodIsPickableAndTinted pins first-class QUERY support (RFC
// 10008): it must appear in the picker's option list, carry its own colour
// rather than the unknown-method fallback, and survive SetSelected. An
// OpenAPI 3.2 `additionalOperations` token like PURGE is deliberately NOT
// offered — those are arbitrary strings, so they import and send but stay off
// the list and render with the neutral fallback.
func TestQueryMethodIsPickableAndTinted(t *testing.T) {
	test.NewApp()

	if !slices.Contains(methodNames(), string(model.QUERY)) {
		t.Fatalf("QUERY missing from the picker options: %v", methodNames())
	}
	if slices.Contains(methodNames(), "PURGE") {
		t.Error("PURGE must not be offered — it is not a registered method")
	}

	fallback := theme.Color(theme.ColorNameForeground)
	if got := methodColor(string(model.QUERY)); got == fallback {
		t.Error("QUERY renders with the unknown-method fallback; it needs its own tint")
	}
	// Its tint must also be distinct from every other method's, or the colour
	// stops carrying information.
	seen := map[color.Color]model.Method{}
	for _, mth := range model.Methods {
		c := methodColor(string(mth))
		if mth == model.TRACE || mth == model.CONNECT {
			continue // documented as deliberately sharing one grey
		}
		if prev, dup := seen[c]; dup {
			t.Errorf("%s and %s share a colour", prev, mth)
		}
		seen[c] = mth
	}

	p := newMethodPicker(func(string) {})
	p.SetSelected(string(model.QUERY))
	if p.selected != string(model.QUERY) {
		t.Errorf("picker selected = %q, want QUERY", p.selected)
	}
}
