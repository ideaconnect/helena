package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

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

// TestSplitPaneRootThemeOverrides covers the override values of the remaining
// scoped sub-themes (splitTheme/paneTheme/rootTheme) and that they delegate
// everything else to the app theme via the embedded delegatingTheme (#56).
func TestSplitPaneRootThemeOverrides(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	ApplyTheme(a, model.ThemeDark)

	// splitTheme: thin divider padding, separator-coloured fill, transparent handle.
	if got := (splitTheme{}).Size(theme.SizeNamePadding); got != 1.5 {
		t.Errorf("split padding = %v; want 1.5", got)
	}
	sep := appTheme().Color(theme.ColorNameSeparator, theme.VariantDark)
	if got := (splitTheme{}).Color(theme.ColorNameShadow, theme.VariantDark); got != sep {
		t.Errorf("split shadow colour = %v; want separator %v", got, sep)
	}
	if got := (splitTheme{}).Color(theme.ColorNameForeground, theme.VariantDark); got != color.Transparent {
		t.Errorf("split foreground (grab handle) = %v; want transparent", got)
	}
	// A non-overridden size/colour delegates.
	if got := (splitTheme{}).Size(theme.SizeNameText); got != appTheme().Size(theme.SizeNameText) {
		t.Error("split theme should delegate non-padding sizes")
	}

	// rootTheme: zero padding, everything else delegated.
	if got := (rootTheme{}).Size(theme.SizeNamePadding); got != 0 {
		t.Errorf("root padding = %v; want 0", got)
	}
	if got := (rootTheme{}).Size(theme.SizeNameText); got != appTheme().Size(theme.SizeNameText) {
		t.Error("root theme should delegate non-padding sizes")
	}

	// paneTheme: pure delegation (the embedded base is the whole implementation).
	if got := (paneTheme{}).Size(theme.SizeNamePadding); got != appTheme().Size(theme.SizeNamePadding) {
		t.Error("pane theme should fully delegate padding")
	}
	if got := (paneTheme{}).Color(theme.ColorNamePrimary, theme.VariantDark); got != appTheme().Color(theme.ColorNamePrimary, theme.VariantDark) {
		t.Error("pane theme should delegate colour")
	}
	if (paneTheme{}).Font(fyne.TextStyle{}) != appTheme().Font(fyne.TextStyle{}) {
		t.Error("pane theme should delegate font")
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
