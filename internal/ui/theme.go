package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/idct/helena/internal/model"
)

// ApplyTheme switches the app's theme according to t. ThemeSystem follows the
// OS appearance (Fyne's default theme respects the system variant).
func ApplyTheme(app fyne.App, t model.Theme) {
	switch t {
	case model.ThemeLight:
		app.Settings().SetTheme(theme.LightTheme())
	case model.ThemeDark:
		app.Settings().SetTheme(theme.DarkTheme())
	default:
		app.Settings().SetTheme(theme.DefaultTheme())
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
