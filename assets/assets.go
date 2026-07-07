// Package assets embeds static files (the application icon, the Font Awesome
// SVGs used across the UI, and the Inter + JetBrains Mono typefaces) so they
// ship inside the single Helena binary. Font Awesome Free icons are CC BY 4.0
// (assets/icons/LICENSE-fontawesome.txt); Inter and JetBrains Mono are
// SIL-OFL-1.1-licensed (assets/fonts/LICENSE-*.txt). See THIRD_PARTY_NOTICES.md.
package assets

import (
	"embed"
	_ "embed"
	"fmt"

	"fyne.io/fyne/v2"
)

// AppIcon is the PNG application/window icon (the Helena cat mascot) at
// 256×256 — plenty for any taskbar/dock. The full-resolution art stays in
// assets/app_icon.png for packaging; embedding it would cost ~1 MB of binary
// and resident memory, and Fyne decodes the window icon on the GL main
// thread at startup, so smaller is measurably faster to first frame.
//
//go:embed app_icon_window.png
var AppIcon []byte

// HelenaCat is a photo of Helena — the cat the app is named after — shown in the
// Help → About dialog. Resized to 341×400 and EXIF-stripped (~40 KB); the full
// original is not kept in-repo.
//
//go:embed helena_cat.jpg
var HelenaCat []byte

// CoffeeIcon is the Buy Me a Coffee brand mark (same icon as the website nav),
// used for the Help → "Buy me a coffee" menu entry. 44×64 PNG (~3 KB).
//
//go:embed bmc_coffee.png
var CoffeeIcon []byte

// iconFS holds every SVG under assets/icons/. Use Icon(name) to fetch
// a Fyne resource ready for widget.NewButtonWithIcon and friends.
//
//go:embed icons/*.svg
var iconFS embed.FS

// fontFS holds the embedded TTFs under assets/fonts/ (Inter for the UI,
// JetBrains Mono for monospace). Use Font(name) to fetch one as a Fyne
// resource for a custom theme's Font(style) method.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

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

// Font returns the TTF resource at fonts/<name>.ttf as a fyne.Resource.
// Panics if the font isn't embedded — like Icon, call sites are static so
// a typo is a build-time bug. Cache the result locally for hot paths.
func Font(name string) fyne.Resource {
	path := "fonts/" + name + ".ttf"
	b, err := fontFS.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("assets: missing font %q (check assets/fonts/): %w", name, err))
	}
	return fyne.NewStaticResource(name+".ttf", b)
}
