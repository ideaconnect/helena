package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
)

func TestThemeNameRoundTrip(t *testing.T) {
	for _, th := range []model.Theme{model.ThemeSystem, model.ThemeLight, model.ThemeDark} {
		if back := themeFromName(themeName(th)); back != th {
			t.Errorf("round-trip %v -> %q -> %v", th, themeName(th), back)
		}
	}
	if themeFromName("unknown") != model.ThemeSystem {
		t.Errorf("unknown label should fall back to System")
	}
}

func TestApplyThemeDoesNotPanic(t *testing.T) {
	a := test.NewApp()
	for _, th := range []model.Theme{model.ThemeSystem, model.ThemeLight, model.ThemeDark, model.Theme("garbage")} {
		ApplyTheme(a, th)
	}
}
