package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// overriddenNames are every colour Helena's theme supplies itself (so they must
// not fall through to the stock blue-accent palette).
var overriddenNames = []fyne.ThemeColorName{
	theme.ColorNameBackground, theme.ColorNameHeaderBackground,
	theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground,
	theme.ColorNameInputBackground, theme.ColorNameInputBorder,
	theme.ColorNameSeparator, theme.ColorNameButton, theme.ColorNameDisabledButton,
	theme.ColorNameHover, theme.ColorNamePressed, theme.ColorNameSelection,
	theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameForegroundOnPrimary,
	theme.ColorNameForeground, theme.ColorNameDisabled, theme.ColorNamePlaceHolder,
	theme.ColorNameHyperlink, theme.ColorNameError, theme.ColorNameSuccess,
	theme.ColorNameWarning, theme.ColorNameShadow, theme.ColorNameScrollBar,
}

// TestHelenaColorAllOverridesPresent ensures every name we claim to own
// returns a colour in both variants — a missing case would silently leak the
// stock palette.
func TestHelenaColorAllOverridesPresent(t *testing.T) {
	for _, v := range []fyne.ThemeVariant{theme.VariantDark, theme.VariantLight} {
		for _, n := range overriddenNames {
			if c, ok := helenaColor(n, v); !ok || c == nil {
				t.Errorf("helenaColor(%s, variant=%d) ok=%v c=%v; want a colour", n, v, ok, c)
			}
		}
	}
}

// TestHelenaColorAccentIsGreen pins the brand: Primary is the Helena green in
// both variants and never Fyne's default blue.
func TestHelenaColorAccentIsGreen(t *testing.T) {
	dark, _ := helenaColor(theme.ColorNamePrimary, theme.VariantDark)
	if dark != (color.NRGBA{0x53, 0xd0, 0x60, 0xff}) {
		t.Errorf("dark primary = %v; want Helena green", dark)
	}
	light, _ := helenaColor(theme.ColorNamePrimary, theme.VariantLight)
	if light != (color.NRGBA{0x2f, 0x9e, 0x54, 0xff}) {
		t.Errorf("light primary = %v; want Helena green", light)
	}
	// Selection is a translucent green wash, not the opaque primary.
	sel, _ := helenaColor(theme.ColorNameSelection, theme.VariantDark)
	if nr := sel.(color.NRGBA); nr.A == 0xff || nr.G < nr.R || nr.G < nr.B {
		t.Errorf("dark selection = %v; want translucent green", sel)
	}
}

// TestHelenaColorDelegatesUnknown verifies a name we do NOT override falls
// through to the embedded base theme rather than returning nil.
func TestHelenaColorDelegatesUnknown(t *testing.T) {
	h := helenaTheme{base: theme.DefaultTheme()}
	got := h.Color(theme.ColorNameScrollBarBackground, theme.VariantDark)
	want := theme.DefaultTheme().Color(theme.ColorNameScrollBarBackground, theme.VariantDark)
	if got != want {
		t.Errorf("delegated color = %v; want base %v", got, want)
	}
}

// TestHelenaThemeForcedVariant verifies Light/Dark pin the served variant
// regardless of the variant Fyne passes (SetTheme does not flip the app
// variant), while System lets the caller's variant through.
func TestHelenaThemeForcedVariant(t *testing.T) {
	darkBg := color.NRGBA{0x1a, 0x1a, 0x1a, 0xff}
	lightBg := color.NRGBA{0xf7, 0xf7, 0xf7, 0xff}

	dark := newHelenaTheme(model.ThemeDark)
	if got := dark.Color(theme.ColorNameBackground, theme.VariantLight); got != darkBg {
		t.Errorf("forced-dark asked Light = %v; want dark bg", got)
	}
	light := newHelenaTheme(model.ThemeLight)
	if got := light.Color(theme.ColorNameBackground, theme.VariantDark); got != lightBg {
		t.Errorf("forced-light asked Dark = %v; want light bg", got)
	}
	sys := newHelenaTheme(model.ThemeSystem)
	if got := sys.Color(theme.ColorNameBackground, theme.VariantDark); got != darkBg {
		t.Errorf("system asked Dark = %v; want dark bg", got)
	}
	if got := sys.Color(theme.ColorNameBackground, theme.VariantLight); got != lightBg {
		t.Errorf("system asked Light = %v; want light bg", got)
	}
}

// TestHelenaThemeFont maps every TextStyle to the expected embedded face and
// leaves the symbol font to the base theme.
func TestHelenaThemeFont(t *testing.T) {
	h := helenaTheme{base: theme.DefaultTheme()}
	cases := []struct {
		style fyne.TextStyle
		want  string
	}{
		{fyne.TextStyle{}, "Inter-Regular.ttf"},
		{fyne.TextStyle{Bold: true}, "Inter-Bold.ttf"},
		{fyne.TextStyle{Italic: true}, "Inter-Italic.ttf"},
		{fyne.TextStyle{Bold: true, Italic: true}, "Inter-BoldItalic.ttf"},
		{fyne.TextStyle{Monospace: true}, "JetBrainsMono-Regular.ttf"},
		{fyne.TextStyle{Monospace: true, Bold: true}, "JetBrainsMono-Bold.ttf"},
	}
	for _, c := range cases {
		if got := h.Font(c.style); got == nil || got.Name() != c.want {
			t.Errorf("Font(%+v) = %v; want %s", c.style, got, c.want)
		}
	}
	symbol := fyne.TextStyle{Symbol: true}
	if got, want := h.Font(symbol).Name(), h.base.Font(symbol).Name(); got != want {
		t.Errorf("Font(symbol) = %s; want base %s", got, want)
	}
}

// TestHelenaThemeSize pins the tuned sizes and confirms delegation otherwise.
func TestHelenaThemeSize(t *testing.T) {
	h := helenaTheme{base: theme.DefaultTheme()}
	if got := h.Size(theme.SizeNameText); got != 13 {
		t.Errorf("text size = %v; want 13", got)
	}
	if got := h.Size(theme.SizeNameInputRadius); got != 6 {
		t.Errorf("input radius = %v; want 6", got)
	}
	if got, want := h.Size(theme.SizeNameInlineIcon), h.base.Size(theme.SizeNameInlineIcon); got != want {
		t.Errorf("delegated size = %v; want base %v", got, want)
	}
}

// TestSidebarAndToolbarThemes verifies the two sidebar-scoped theme overrides
// only change the inline-icon (and, for the tree, padding) sizes and delegate
// everything else to the live app theme.
func TestSidebarAndToolbarThemes(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	ApplyTheme(a, model.ThemeDark)

	if got := (sidebarTheme{}).Size(theme.SizeNameInlineIcon); got != 14 {
		t.Errorf("sidebar inline icon = %v; want 14", got)
	}
	if got := (toolbarTheme{}).Size(theme.SizeNameInlineIcon); got != 24 {
		t.Errorf("toolbar inline icon = %v; want 24", got)
	}
	// A non-overridden size delegates to the app theme.
	want := appTheme().Size(theme.SizeNameText)
	if got := (toolbarTheme{}).Size(theme.SizeNameText); got != want {
		t.Errorf("toolbar delegated text size = %v; want %v", got, want)
	}
	// Colour/font delegate to the app theme (green accent, Inter).
	if got := (toolbarTheme{}).Color(theme.ColorNamePrimary, theme.VariantDark); got != appTheme().Color(theme.ColorNamePrimary, theme.VariantDark) {
		t.Error("toolbar theme should delegate colour to the app theme")
	}
}

// TestThinSplitDividerStyling pins the hairline divider that replaced the old
// splitTheme/paneTheme scope overrides: 3px thickness, separator-coloured idle
// fill, hover fill while hovered, no centre grab handle, and resize cursors.
func TestThinSplitDividerStyling(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	ApplyTheme(a, model.ThemeDark)

	if thinDividerThickness != 3 {
		t.Errorf("divider thickness = %v; want 3 (old splitTheme padding 1.5 x 2)", thinDividerThickness)
	}

	h := newThinSplit(true, widget.NewLabel("L"), widget.NewLabel("R"))
	d := newThinDivider(h)
	r := d.CreateRenderer().(*thinDividerRenderer)
	r.Refresh()
	sep := appTheme().Color(theme.ColorNameSeparator, theme.VariantDark)
	if r.line.FillColor != sep {
		t.Errorf("idle divider fill = %v; want separator %v", r.line.FillColor, sep)
	}
	d.hovered = true
	r.Refresh()
	hov := appTheme().Color(theme.ColorNameHover, theme.VariantDark)
	if r.line.FillColor != hov {
		t.Errorf("hovered divider fill = %v; want hover %v", r.line.FillColor, hov)
	}
	if len(r.Objects()) != 1 {
		t.Errorf("divider renders %d objects; want 1 (no grab handle)", len(r.Objects()))
	}
	if d.Cursor() != desktop.HResizeCursor {
		t.Error("horizontal split divider should use the H-resize cursor")
	}
	v := newThinSplit(false, widget.NewLabel("T"), widget.NewLabel("B"))
	if newThinDivider(v).Cursor() != desktop.VResizeCursor {
		t.Error("vertical split divider should use the V-resize cursor")
	}
}

// TestThinSplitLayoutRespectsOffset verifies the split's layout math: panes
// share the space around the 3px divider at the requested offset.
func TestThinSplitLayoutRespectsOffset(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	s := newThinSplit(true, widget.NewLabel(""), widget.NewLabel(""))
	s.SetOffset(0.25)
	w := test.NewWindow(s)
	defer w.Close()
	w.Resize(fyne.NewSize(403, 100))
	// 403 - 3 divider = 400 free; offset 0.25 -> leading 100 (within min-size
	// clamps for empty labels).
	lw := s.Leading.Size().Width
	if lw < 95 || lw > 105 {
		t.Errorf("leading width = %v; want ~100 at offset 0.25 of 400 free", lw)
	}
	if got := s.Leading.Size().Width + thinDividerThickness + s.Trailing.Size().Width; got > s.Size().Width+1 {
		t.Errorf("panes + divider (%v) overflow the split (%v)", got, s.Size().Width)
	}
}

// TestFlushColumnLayout pins the zero-spacing column that replaced the old
// rootTheme zero-padding scope: fixed rows get their min height, the flex row
// takes the remainder, and every row is flush against the next.
func TestFlushColumnLayout(t *testing.T) {
	mk := func(minH float32) fyne.CanvasObject {
		r := canvas.NewRectangle(color.Black)
		r.SetMinSize(fyne.NewSize(10, minH))
		return r
	}
	objects := []fyne.CanvasObject{mk(20), mk(1), mk(5), mk(1), mk(24)}
	l := &flushColumn{flexIdx: 2}
	l.Layout(objects, fyne.NewSize(300, 200))

	wantY := []float32{0, 20, 21, 175, 176}
	wantH := []float32{20, 1, 154, 1, 24}
	for i, o := range objects {
		if o.Position().Y != wantY[i] {
			t.Errorf("row %d y = %v; want %v", i, o.Position().Y, wantY[i])
		}
		if o.Size().Height != wantH[i] {
			t.Errorf("row %d height = %v; want %v", i, o.Size().Height, wantH[i])
		}
		if o.Size().Width != 300 {
			t.Errorf("row %d width = %v; want full 300", i, o.Size().Width)
		}
	}
	if got := l.MinSize(objects); got.Height != 51 || got.Width != 10 {
		t.Errorf("MinSize = %v; want 10x51 (sum of min heights)", got)
	}
}

// TestThemedIconReturnsThemedResource verifies themedIcon wraps an embedded SVG
// into a non-nil, named resource (so toolbar icons recolour to the foreground).
func TestThemedIconReturnsThemedResource(t *testing.T) {
	r := themedIcon("circle-xmark")
	if r == nil || r.Name() == "" || len(r.Content()) == 0 {
		t.Errorf("themedIcon returned an empty resource: %+v", r)
	}
}

// TestApplyThemeInstallsHelena verifies ApplyTheme actually installs the custom
// theme (not a stock one) for every setting.
func TestApplyThemeInstallsHelena(t *testing.T) {
	a := test.NewApp()
	for _, th := range []model.Theme{model.ThemeSystem, model.ThemeLight, model.ThemeDark} {
		ApplyTheme(a, th)
		if _, ok := a.Settings().Theme().(helenaTheme); !ok {
			t.Errorf("ApplyTheme(%v) installed %T; want helenaTheme", th, a.Settings().Theme())
		}
	}
}

// TestThemedIconMemoizes: repeated lookups of one icon name must return the
// same resource instance (construction requests ~21 icons with duplicates;
// each miss re-reads and re-wraps the embedded file).
func TestThemedIconMemoizes(t *testing.T) {
	if themedIcon("play") != themedIcon("play") {
		t.Error("themedIcon should return the memoized instance on repeat lookups")
	}
}
