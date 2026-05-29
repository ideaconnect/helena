# cmd/helena

The Helena binary entrypoint. This package contains a single `main.go` that
wires together the application icon, the persisted session, the Fyne theme,
and the main UI, then shows the primary window and blocks on the Fyne main
loop. There is no business logic here — everything interesting lives in
`internal/`.

The package is named `main` and produces the `helena` executable when built
with `go build ./cmd/helena/`.

## Entry point

- [main.go](main.go) — `func main()`. See STRUCTURE.md and WORKFLOW.md for
  the startup sequence.

## Dependencies

- `fyne.io/fyne/v2` and `fyne.io/fyne/v2/app` — windowing and app lifecycle.
- `github.com/idct/helena/assets` — embedded application icon
  (`assets.AppIcon`).
- `github.com/idct/helena/internal/config` — resolves the platform-specific
  config file location (`config.DefaultPath`).
- `github.com/idct/helena/internal/session` — loads / saves workspaces,
  collections, settings, and the last-open request and window size.
- `github.com/idct/helena/internal/ui` — the Fyne view layer (`ui.NewMainUI`,
  `ui.ApplyTheme`).
