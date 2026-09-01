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
// Results are memoized: several icons are requested repeatedly (construction
// alone asks for ~21, with duplicates), and each miss re-reads + re-wraps the
// embedded file. A ThemedResource adapts to theme changes at render time, so
// sharing one instance across callers is safe. UI-goroutine only.
func themedIcon(name string) fyne.Resource {
	if r, ok := themedIcons[name]; ok {
		return r
	}
	r := theme.NewThemedResource(assets.Icon(name))
	themedIcons[name] = r
	return r
}

var themedIcons = map[string]fyne.Resource{}

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

// appTheme returns the currently installed app theme, or the stock theme if no
// app is running (defensive — the sub-themes below delegate everything but a
// few Size/Color overrides to it).
func appTheme() fyne.Theme {
	if a := fyne.CurrentApp(); a != nil {
		if th := a.Settings().Theme(); th != nil {
			return th
		}
	}
	return theme.DefaultTheme()
}

// delegatingTheme implements fyne.Theme by passing every method through to the
// live app theme (appTheme). The scoped sub-themes below embed it and override
// only the Color/Size cases they actually change, so the pass-through
// boilerplate lives in exactly one place. It is stateless (appTheme reads the
// global), so embedding it by value is fine; its methods call appTheme directly
// rather than each other, so a sub-theme's Size override is never bypassed.
type delegatingTheme struct{}

func (delegatingTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return appTheme().Color(n, v)
}
func (delegatingTheme) Font(s fyne.TextStyle) fyne.Resource     { return appTheme().Font(s) }
func (delegatingTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return appTheme().Icon(n) }
func (delegatingTheme) Size(n fyne.ThemeSizeName) float32       { return appTheme().Size(n) }

// sidebarTheme tightens the collections tree. widget.Tree multiplies
// SizeNameInlineIcon + SizeNamePadding into its per-level indentation (see
// treeNode.Indent), so shrinking those two sizes makes nesting noticeably
// shallower and the disclosure/action icons denser. It is applied only to the
// tree subtree via container.NewThemeOverride, so the rest of the app keeps
// full-size icons. Only Size differs (no variant to keep in sync); colour/font/
// icon delegate via the embedded base so the sidebar still tracks Light/Dark.
type sidebarTheme struct{ delegatingTheme }

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
// buttons are chunky and easy to hit. Like sidebarTheme it only overrides Size.
type toolbarTheme struct{ delegatingTheme }

func (toolbarTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameInlineIcon {
		return 24
	}
	return appTheme().Size(n)
}

// tabsTheme restores the 4pt active-tab indicator. Fyne 2.8 changed AppTabs to
// size the selected-tab underline from SizeNameSeparatorThickness instead of
// SizeNamePadding (container/apptabs.go: indicatorSize = NewSize(w,
// dividerWidth)); helenaTheme pins SeparatorThickness to 1 for hairline
// separators, which silently collapsed the green accent pill to a 1pt line.
// Overriding the size only inside the tab subtrees keeps the hairline
// everywhere else. Scoped rather than global because widget.NewSeparator is
// used structurally in the toolbar, sidebar and status bar, where 4pt would be
// a heavy rule instead of a hairline.
type tabsTheme struct{ delegatingTheme }

func (tabsTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameSeparatorThickness {
		return 4 // the pre-2.8 indicator height (Fyne's SizeNamePadding default)
	}
	return appTheme().Size(n)
}

// The splits' thin divider and the root's flush hairline joins used to be
// splitTheme / paneTheme / rootTheme scope overrides here; they are now the
// scope-free thinSplit widget ([thinsplit.go](thinsplit.go)) and flushColumn
// layout ([helpers.go](helpers.go)) — each container.NewThemeOverride call
// mints a Fyne theme scope, and Fyne re-parses fonts per scope.

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
