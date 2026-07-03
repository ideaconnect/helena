# assets — structure

## Files

| File | Purpose |
| ---- | ------- |
| [assets.go](assets.go) | Package declaration, the `import _ "embed"` line that enables the directive, the `//go:embed app_icon_window.png` directive (the 256×256 window icon — the full-res `app_icon.png` stays unembedded for packaging), and the `AppIcon []byte` variable. |
| `app_icon_window.png` | The 256×256 PNG embedded into `AppIcon` (~92 KB). |
| `app_icon.png` | The full-resolution (896×896) mascot art, kept for packaging/marketing. Not embedded — shipping it cost ~1 MB of binary and resident memory for pixels no dock ever shows. |

## Bundled artifact

- `app_icon_window.png` (the 256×256 downscale of the Helena cat mascot) is
  the only file consumed at compile time by the `//go:embed` directive and
  the only file the package exposes via `AppIcon`. It is **not** modified at
  runtime — the `[]byte` slice points into the binary's read-only data.

## How it is consumed

The single caller is `cmd/helena/main.go`:

```go
icon := fyne.NewStaticResource("app_icon_window.png", assets.AppIcon)
a.SetIcon(icon)
// ...
w.SetIcon(icon)
```

`fyne.NewStaticResource` wraps the byte slice into a `fyne.Resource` that
Fyne can hand to the OS for app/window decoration. Replacing the icon means
dropping the new art at `assets/app_icon.png`, regenerating the embedded
downscale (`convert app_icon.png -resize 256x256 -strip app_icon_window.png`),
and rebuilding — no code changes required.
