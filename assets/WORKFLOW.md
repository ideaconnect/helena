# assets — workflows

## Embedding the app icon at compile time

There is no runtime flow: this package's job is finished by the Go
toolchain before `main` ever runs.

1. **Source declaration.** [assets.go](assets.go) carries the
   `//go:embed app_icon.png` directive immediately above
   `var AppIcon []byte`. The `import _ "embed"` blank import on the line
   above pulls in the runtime support needed for the directive to be
   recognised.
2. **Compile time.** When `go build` processes the package, the toolchain
   reads `app_icon.png` from the package directory and bakes its bytes
   into the binary's read-only data section. The resulting `AppIcon`
   variable is a slice that points into that section. No file I/O happens
   at runtime.
3. **Runtime consumption.** `cmd/helena/main.go` reads `AppIcon` during
   `main()`:
   - `icon := fyne.NewStaticResource("app_icon.png", assets.AppIcon)`
     wraps the bytes in a Fyne `Resource` with a logical name.
   - `a.SetIcon(icon)` and `w.SetIcon(icon)` hand the resource to the OS
     for app-launcher and window-decoration use.

Because the bytes are embedded, the shipped binary has no external icon
dependency. To replace the icon, overwrite `assets/app_icon.png` and
rebuild — the directive picks up the new file on the next compile.
