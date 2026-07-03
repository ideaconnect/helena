# assets

Static resources embedded into the Helena binary at compile time: the
application icon, the Font Awesome SVGs used across the UI, and the Inter +
JetBrains Mono typefaces served by Helena's custom theme. Everything here
ships *inside* the single binary — there is no runtime asset directory.

## Files

- [assets.go](assets.go) — package declaration, the three `//go:embed`
  directives (`AppIcon`, `iconFS`, `fontFS`), and the `Icon` / `Font`
  accessors.
- `app_icon.png` — the full-resolution (896×896) Helena cat mascot, kept for
  packaging/marketing art. **Not embedded.**
- `app_icon_window.png` — the 256×256 downscale embedded into `AppIcon` and
  used as the app/taskbar/window icon. 256px covers every dock; embedding the
  full-res art cost ~1 MB of binary + resident memory, and Fyne decodes the
  window icon on the GL main thread at startup.
- `icons/*.svg` — the Font Awesome Free glyphs, fetched by name via `Icon`
  (each carries `fill="currentColor"` so it adopts the theme colour).
- `icons/LICENSE-fontawesome.txt` — Font Awesome Free license/attribution (icons
  are CC BY 4.0).
- `fonts/*.ttf` — Inter (`Inter-Regular/Bold/Italic/BoldItalic`) for UI text
  and JetBrains Mono (`JetBrainsMono-Regular/Bold`) for monospace, fetched by
  name via `Font`.
- `fonts/LICENSE-Inter.txt`, `fonts/LICENSE-JetBrainsMono.txt` — the fonts'
  SIL OFL 1.1 licenses (also reproduced in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)).

To replace the icon swap `app_icon.png` in place and regenerate the embedded
downscale with `convert app_icon.png -resize 256x256 -strip app_icon_window.png`;
to add an icon or font, drop the file into `icons/` or `fonts/` and reference it
by name. The embedded bytes pick up the new file the next time the binary is
built.

## Public API

- `AppIcon []byte` — the PNG bytes of the 256×256 application/window icon,
  loaded via `//go:embed app_icon_window.png`.
- `Icon(name string) fyne.Resource` — returns `icons/<name>.svg` as a Fyne
  resource ready for `widget.NewButtonWithIcon` and friends. Panics on a
  missing name (call sites are static, so a typo is a build-time bug).
- `Font(name string) fyne.Resource` — returns `fonts/<name>.ttf` as a Fyne
  resource for a custom theme's `Font(style)` method. Panics on a missing
  name, like `Icon`.

## Dependencies

- The standard library's `embed` package (used via `import "embed"` to enable
  the `//go:embed` directives).
- `fyne.io/fyne/v2` for the `fyne.Resource` return type / `fyne.NewStaticResource`.

## Licensing

Font Awesome Free icons are CC BY 4.0; Inter and JetBrains Mono are
SIL-OFL-1.1-licensed. The texts live alongside the assets
(`icons/LICENSE-fontawesome.txt`, `fonts/LICENSE-*.txt`) and in
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).
