package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/idct/helena/internal/model"
)

// ApplyTheme installs Helena's custom theme (see helena_theme.go) pinned to the
// variant for t. ThemeLight/ThemeDark force that variant; ThemeSystem follows
// the OS appearance. The custom theme embeds the stock theme, so anything it
// doesn't override behaves as before.
func ApplyTheme(app fyne.App, t model.Theme) {
	app.Settings().SetTheme(newHelenaTheme(t))
}

// variantFor maps a Helena Theme to the Fyne theme variant PrettyView keys its
// palette on. ThemeSystem defers to whatever variant the running app reports.
// Needed because ApplyTheme swaps the whole theme object without flipping the
// app's variant, so PrettyView must be told the variant explicitly to repaint.
func variantFor(t model.Theme) fyne.ThemeVariant {
	switch t {
	case model.ThemeLight:
		return theme.VariantLight
	case model.ThemeDark:
		return theme.VariantDark
	default:
		return fyne.CurrentApp().Settings().ThemeVariant()
	}
}

// themeName maps a Theme enum to the user-facing label shown in the picker.
func themeName(t model.Theme) string {
	switch t {
	case model.ThemeLight:
		return "Light"
	case model.ThemeDark:
		return "Dark"
	default:
		return "System"
	}
}

// themeFromName is the inverse of themeName; unknown labels fall back to System.
func themeFromName(name string) model.Theme {
	switch name {
	case "Light":
		return model.ThemeLight
	case "Dark":
		return model.ThemeDark
	default:
		return model.ThemeSystem
	}
}
