# assets — structure

## Files

| File | Purpose |
| ---- | ------- |
| [assets.go](assets.go) | Package declaration, the `import _ "embed"` line that enables the directive, the `//go:embed app_icon.png` directive, and the `AppIcon []byte` variable. |
| `app_icon.png` | The PNG bytes embedded into `AppIcon`. Roughly 1.7 MB; this is intentional — the file ships baked sizes so the OS can pick an appropriate one without runtime resizing. |

## Bundled artifact

- `app_icon.png` is the Helena cat mascot. It is the only file consumed at
  compile time by the `//go:embed` directive and the only file the package
  exposes via `AppIcon`. It is **not** modified at runtime — the
  `[]byte` slice points into the binary's read-only data.

## How it is consumed

The single caller is `cmd/helena/main.go`:

```go
icon := fyne.NewStaticResource("app_icon.png", assets.AppIcon)
a.SetIcon(icon)
// ...
w.SetIcon(icon)
```

`fyne.NewStaticResource` wraps the byte slice into a `fyne.Resource` that
Fyne can hand to the OS for app/window decoration. Replacing the icon is a
matter of dropping a new PNG at `assets/app_icon.png` and rebuilding — no
code changes required.
