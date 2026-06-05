package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/idct/helena/assets"
	"github.com/idct/helena/internal/model"
)

// helenaTheme is Helena's custom Fyne theme. It replaces Fyne's generic
// blue-accent stock look with:
//
//   - a layered surface ramp (canvas → menu/selection → header chrome →
//     borders) so panels read as deliberate elevation steps rather than one
//     flat grey;
//   - a single restrained green brand accent (derived from the GET method
//     colour) reserved for the primary/active/focus state;
//   - denser sizing (13px base text, 6px corner radius, thin separators);
//   - the embedded Inter (UI) and JetBrains Mono (monospace) typefaces.
//
// It embeds the stock theme and delegates any colour/size/icon name it does
// not override, so future Fyne theme names keep working.
//
// forced pins the served variant. Fyne's Settings().SetTheme swaps the whole
// theme object but does NOT flip the app's light/dark variant, so to honour an
// explicit Light/Dark choice we must ignore the variant passed to Color and
// serve our own. nil means "follow the caller" — used for ThemeSystem so the
// app tracks the OS appearance.
type helenaTheme struct {
	base   fyne.Theme
	forced *fyne.ThemeVariant
}

// newHelenaTheme builds the theme for a Helena Theme setting. Light/Dark pin
// the variant; System lets the base variant pass through.
func newHelenaTheme(t model.Theme) fyne.Theme {
	h := helenaTheme{base: theme.DefaultTheme()}
	switch t {
	case model.ThemeLight:
		v := theme.VariantLight
		h.forced = &v
	case model.ThemeDark:
		v := theme.VariantDark
		h.forced = &v
	}
	return h
}

// themedIcon wraps an embedded SVG in a ThemedResource so Fyne recolours it to
// the current theme's foreground (the "slightly gray" used everywhere). This is
// load-bearing: Fyne only recolours button icons for high/danger/etc.
// importance — for medium/low-importance buttons a plain static resource is
// drawn as-is, so the currentColor SVGs would render black. As a ThemedResource
// the icon is recoloured to the foreground on medium/low buttons and to the
// button's foreground (e.g. ForegroundOnPrimary) on high-importance ones.
func themedIcon(name string) fyne.Resource {
	return theme.NewThemedResource(assets.Icon(name))
}

// Cached font resources. assets.Font panics on a missing name, so a typo is a
// build/load-time failure rather than a per-call cost.
var (
	fontUIRegular    = assets.Font("Inter-Regular")
	fontUIBold       = assets.Font("Inter-Bold")
	fontUIItalic     = assets.Font("Inter-Italic")
	fontUIBoldItalic = assets.Font("Inter-BoldItalic")
	fontMono         = assets.Font("JetBrainsMono-Regular")
	fontMonoBold     = assets.Font("JetBrainsMono-Bold")
)

func (h helenaTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if h.forced != nil {
		v = *h.forced
	}
	if c, ok := helenaColor(name, v); ok {
		return c
	}
	return h.base.Color(name, v)
}

// Font returns Inter for UI text (regular/bold/italic/bold-italic) and
// JetBrains Mono for monospace. The symbol font (used for emoji / icon glyphs
// rendered in text) is left to the base theme.
func (h helenaTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Symbol {
		return h.base.Font(s)
	}
	if s.Monospace {
		if s.Bold {
			return fontMonoBold
		}
		return fontMono
	}
	switch {
	case s.Bold && s.Italic:
		return fontUIBoldItalic
	case s.Bold:
		return fontUIBold
	case s.Italic:
		return fontUIItalic
	default:
		return fontUIRegular
	}
}

func (h helenaTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return h.base.Icon(name)
}

// sidebarTheme tightens the collections tree. widget.Tree multiplies
// SizeNameInlineIcon + SizeNamePadding into its per-level indentation (see
// treeNode.Indent), so shrinking those two sizes makes nesting noticeably
// shallower and the disclosure/action icons denser. It is applied only to the
// tree subtree via container.NewThemeOverride, so the rest of the app keeps
// full-size icons. Colour/font/icon delegate to the live app theme so the
// sidebar still tracks Light/Dark switches with no rebuild; only Size differs,
// and Size has no variant to keep in sync.
type sidebarTheme struct{}

// appTheme returns the currently installed app theme, or the stock theme if no
// app is running (defensive — sidebarTheme delegates everything but Size here).
func appTheme() fyne.Theme {
	if a := fyne.CurrentApp(); a != nil {
		if th := a.Settings().Theme(); th != nil {
			return th
		}
	}
	return theme.DefaultTheme()
}

func (sidebarTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return appTheme().Color(n, v)
}
func (sidebarTheme) Font(s fyne.TextStyle) fyne.Resource { return appTheme().Font(s) }
func (sidebarTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return appTheme().Icon(n)
}
func (sidebarTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInlineIcon:
		return 14 // vs 20 default — the dominant per-level indent term
	case theme.SizeNamePadding:
		return 2 // vs 4 default — tightens indent and row internals
	}
	return appTheme().Size(n)
}

// toolbarTheme enlarges the sidebar action toolbar's icons to 24px so the
// buttons are chunky and easy to hit. Like sidebarTheme it only overrides sizes
// and delegates colour/font/icon to the live app theme.
type toolbarTheme struct{}

func (toolbarTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return appTheme().Color(n, v)
}
func (toolbarTheme) Font(s fyne.TextStyle) fyne.Resource { return appTheme().Font(s) }
func (toolbarTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return appTheme().Icon(n)
}
func (toolbarTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameInlineIcon {
		return 24
	}
	return appTheme().Size(n)
}

// splitTheme restyles a container.Split's divider into a thin, subtle line (like
// VS Code / Bruno) instead of Fyne's thick shadow bar with a centre grab
// handle: it shrinks SizeNamePadding (the divider is padding*2 thick), recolours
// the idle fill (ColorNameShadow) to the subtle separator colour, and hides the
// handle (ColorNameForeground → transparent). Applied ONLY to the split via
// container.NewThemeOverride; each pane is re-wrapped in paneTheme so the shrunk
// padding doesn't cascade into the pane contents. (dividerTheme uses the
// divider's own Theme(), so the override does reach it.)
type splitTheme struct{}

func (splitTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameShadow:
		return appTheme().Color(theme.ColorNameSeparator, v) // the divider line itself
	case theme.ColorNameForeground:
		return color.Transparent // no centre grab handle
	}
	return appTheme().Color(n, v)
}
func (splitTheme) Font(s fyne.TextStyle) fyne.Resource     { return appTheme().Font(s) }
func (splitTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return appTheme().Icon(n) }
func (splitTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNamePadding {
		return 1.5 // divider thickness = padding*2 = 3px: a thin, still-grabbable line
	}
	return appTheme().Size(n)
}

// paneTheme delegates everything to the live app theme. It restores normal
// sizing inside a splitTheme-wrapped split's panes (whose subtree would
// otherwise inherit the split's shrunk padding).
type paneTheme struct{}

func (paneTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return appTheme().Color(n, v)
}
func (paneTheme) Font(s fyne.TextStyle) fyne.Resource     { return appTheme().Font(s) }
func (paneTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return appTheme().Icon(n) }
func (paneTheme) Size(n fyne.ThemeSizeName) float32       { return appTheme().Size(n) }

// rootTheme zeroes SizeNamePadding so the root Border puts no gap between its
// centre (the body / split) and the header/footer rows — the vertical split
// divider then meets the header and footer separator lines flush, with no gap.
// (Border's outer edges are already flush; only the centre↔border gap is
// padding.) Cascades to root-level children, which restore their own padding
// via their own overrides (toolbar → toolbarTheme, split panes → paneTheme,
// status → paneTheme).
type rootTheme struct{}

func (rootTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return appTheme().Color(n, v)
}
func (rootTheme) Font(s fyne.TextStyle) fyne.Resource     { return appTheme().Font(s) }
func (rootTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return appTheme().Icon(n) }
func (rootTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNamePadding {
		return 0
	}
	return appTheme().Size(n)
}

// Size overrides a curated subset for a denser, more modern feel and delegates
// everything else (padding, icon sizes, line spacing, window-button metrics)
// to the base theme.
func (h helenaTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameInputRadius:
		return 6
	case theme.SizeNameSelectionRadius:
		return 6
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarRadius:
		return 4
	}
	return h.base.Size(name)
}

// helenaColor returns Helena's colour for name in the given variant, or
// ok=false to fall back to the base theme. The accent (Primary/Focus/
// Selection/ForegroundOnPrimary) and Hyperlink MUST be returned explicitly:
// the builtin theme derives them from the user's chosen primary colour, so
// delegating would reintroduce Fyne's blue.
func helenaColor(name fyne.ThemeColorName, v fyne.ThemeVariant) (color.Color, bool) {
	if v == theme.VariantLight {
		return helenaLight(name)
	}
	return helenaDark(name)
}

func helenaDark(name fyne.ThemeColorName) (color.Color, bool) {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{0x1a, 0x1a, 0x1a, 0xff}, true // canvas base
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{0x26, 0x29, 0x2b, 0xff}, true // toolbar / tab strip / sidebar header
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return color.NRGBA{0x22, 0x22, 0x24, 0xff}, true // popups / dialogs
	case theme.ColorNameInputBackground:
		return color.NRGBA{0x22, 0x22, 0x24, 0xff}, true
	case theme.ColorNameInputBorder:
		return color.NRGBA{0x44, 0x44, 0x44, 0xff}, true
	case theme.ColorNameSeparator:
		return color.NRGBA{0x33, 0x33, 0x33, 0xff}, true
	case theme.ColorNameButton, theme.ColorNameDisabledButton:
		// Same background for enabled and disabled buttons — a disabled button
		// reads only by its greyed icon/label (ColorNameDisabled), not a darker
		// box, so an icon toolbar with mixed states stays visually consistent.
		return color.NRGBA{0x2b, 0x2e, 0x30, 0xff}, true
	case theme.ColorNameHover:
		return color.NRGBA{0xff, 0xff, 0xff, 0x14}, true // subtle lighten, neutral
	case theme.ColorNamePressed:
		return color.NRGBA{0xff, 0xff, 0xff, 0x1f}, true
	case theme.ColorNameSelection:
		return color.NRGBA{0x53, 0xd0, 0x60, 0x33}, true // green wash
	case theme.ColorNamePrimary:
		return color.NRGBA{0x53, 0xd0, 0x60, 0xff}, true // Helena green
	case theme.ColorNameFocus:
		return color.NRGBA{0x53, 0xd0, 0x60, 0x8c}, true
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{0x10, 0x18, 0x12, 0xff}, true // near-black on green
	case theme.ColorNameForeground:
		return color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}, true
	case theme.ColorNameDisabled:
		return color.NRGBA{0x5a, 0x5a, 0x5a, 0xff}, true
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{0x99, 0x99, 0x99, 0xff}, true
	case theme.ColorNameHyperlink:
		return color.NRGBA{0x8c, 0xc4, 0xff, 0xff}, true // links stay blue, not green
	case theme.ColorNameError:
		return color.NRGBA{0xd9, 0x48, 0x3b, 0xff}, true
	case theme.ColorNameSuccess:
		return color.NRGBA{0x74, 0xe2, 0x9b, 0xff}, true
	case theme.ColorNameWarning:
		return color.NRGBA{0xf0, 0xa8, 0x68, 0xff}, true
	case theme.ColorNameShadow:
		return color.NRGBA{0x00, 0x00, 0x00, 0x80}, true
	case theme.ColorNameScrollBar:
		return color.NRGBA{0xff, 0xff, 0xff, 0x33}, true
	}
	return nil, false
}

func helenaLight(name fyne.ThemeColorName) (color.Color, bool) {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{0xf7, 0xf7, 0xf7, 0xff}, true
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{0xec, 0xec, 0xec, 0xff}, true
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}, true
	case theme.ColorNameInputBackground:
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}, true
	case theme.ColorNameInputBorder:
		return color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}, true
	case theme.ColorNameSeparator:
		return color.NRGBA{0xdd, 0xdd, 0xdd, 0xff}, true
	case theme.ColorNameButton, theme.ColorNameDisabledButton:
		return color.NRGBA{0xe8, 0xe8, 0xe8, 0xff}, true
	case theme.ColorNameHover:
		return color.NRGBA{0x00, 0x00, 0x00, 0x0d}, true
	case theme.ColorNamePressed:
		return color.NRGBA{0x00, 0x00, 0x00, 0x1f}, true
	case theme.ColorNameSelection:
		return color.NRGBA{0x2f, 0x9e, 0x54, 0x26}, true
	case theme.ColorNamePrimary:
		return color.NRGBA{0x2f, 0x9e, 0x54, 0xff}, true
	case theme.ColorNameFocus:
		return color.NRGBA{0x2f, 0x9e, 0x54, 0x8c}, true
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}, true
	case theme.ColorNameForeground:
		return color.NRGBA{0x1f, 0x1f, 0x1f, 0xff}, true
	case theme.ColorNameDisabled:
		return color.NRGBA{0xb0, 0xb0, 0xb0, 0xff}, true
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{0x88, 0x88, 0x88, 0xff}, true
	case theme.ColorNameHyperlink:
		return color.NRGBA{0x25, 0x63, 0xc9, 0xff}, true
	case theme.ColorNameError:
		return color.NRGBA{0xc0, 0x39, 0x2b, 0xff}, true
	case theme.ColorNameSuccess:
		return color.NRGBA{0x2e, 0x9e, 0x57, 0xff}, true
	case theme.ColorNameWarning:
		return color.NRGBA{0xd9, 0x8b, 0x3a, 0xff}, true
	case theme.ColorNameShadow:
		return color.NRGBA{0x00, 0x00, 0x00, 0x33}, true
	case theme.ColorNameScrollBar:
		return color.NRGBA{0x00, 0x00, 0x00, 0x40}, true
	}
	return nil, false
}
