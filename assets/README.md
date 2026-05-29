# assets

Static resources embedded into the Helena binary at compile time. Currently
that means a single file: `app_icon.png`, the Helena cat mascot, exposed as
`assets.AppIcon` for use as both the macOS dock / Windows taskbar app icon
and the in-window title-bar icon.

No regeneration step exists — to replace the icon, swap `app_icon.png` in
place (PNG, ideally with several baked sizes). The exported `[]byte`
variable will pick up the new bytes the next time the binary is built.

## Files

- [assets.go](assets.go) — package declaration plus the `//go:embed`
  directive and the `AppIcon` variable.
- `app_icon.png` — the PNG bytes embedded into `AppIcon`.

## Public API

- `AppIcon []byte` — the PNG bytes of the application/window icon, loaded at
  compile time via `//go:embed app_icon.png`.

## Dependencies

- The standard library's `embed` package (used via `import _ "embed"` to
  enable the `//go:embed` directive).
