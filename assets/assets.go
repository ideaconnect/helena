// Package assets embeds static files (the application icon and the
// iconoir SVGs used across the UI) so they ship inside the single
// Helena binary. Iconoir is MIT-licensed; full text in
// assets/icons/LICENSE-iconoir.md.
package assets

import (
	"embed"
	_ "embed"
	"fmt"

	"fyne.io/fyne/v2"
)

// AppIcon is the PNG application/window icon (the Helena cat mascot).
//
//go:embed app_icon.png
var AppIcon []byte

// iconFS holds every SVG under assets/icons/. Use Icon(name) to fetch
// a Fyne resource ready for widget.NewButtonWithIcon and friends.
//
//go:embed icons/*.svg
var iconFS embed.FS

// Icon returns the SVG resource at icons/<name>.svg as a fyne.Resource.
// Panics if the icon isn't embedded — call sites are static so a typo
// is a build-time-fixable bug, not a runtime concern. Resources are
// constructed on demand; for hot paths cache the result locally.
func Icon(name string) fyne.Resource {
	path := "icons/" + name + ".svg"
	b, err := iconFS.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("assets: missing icon %q (check assets/icons/): %w", name, err))
	}
	return fyne.NewStaticResource(name+".svg", b)
}
