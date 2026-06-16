# cmd/helena — structure

## Files

| File | Purpose |
| ---- | ------- |
| [main.go](main.go) | Declares `package main`, the `appID` constant (matched against `FyneApp.toml`), the build-metadata vars (`version`/`commit`/`date`, injected via `-ldflags -X`), `versionString`/`windowTitle` helpers, and `func main()`. |
| [main_test.go](main_test.go) | Tests for `versionString` (formatting, commit-trim), `windowTitle`, and the `FyneApp.toml` ID drift guard. |

## Version / build metadata

`version` (default `"dev"`), `commit`, and `date` are package vars set at
release time by the linker (`go build -ldflags "-X main.version=v0.1.0 -X
main.commit=$SHA"`); ci.yml injects `github.ref_name` + `github.sha` (#26).
`helena --version` (or `-version`) prints `versionString(...)` and exits before
any UI is created. A non-dev `version` is also suffixed onto the window title
via `windowTitle`.

## Startup sequence

`func main()` in [main.go](main.go) is short and strictly procedural. Each
step matters because some of them must happen before others can succeed.

0. **`--version` short-circuit** — if the first arg is `--version`/`-version`,
   print the build metadata and return before touching Fyne.
1. **Create the Fyne app** —
   `a := app.NewWithID("tech.idct.helena")` ([main.go:17](main.go#L17)).
   The app ID determines the platform-specific preferences directory Fyne
   uses internally (separate from Helena's own config).
2. **Install the icon** —
   `icon := fyne.NewStaticResource("app_icon.png", assets.AppIcon)` then
   `a.SetIcon(icon)` ([main.go:18-19](main.go#L18-L19)). `assets.AppIcon` is
   the bytes of `assets/app_icon.png` embedded at compile time.
3. **Resolve the config path** —
   `cfgPath, err := config.DefaultPath()`
   ([main.go:21-25](main.go#L21-L25)). On error, log + fall back to
   in-memory persistence (`cfgPath = ""`).
4. **Load the session** —
   `sess, err := session.New(cfgPath)` ([main.go:26-30](main.go#L26-L30)).
   On error, log + retry with `session.New("")` to start fresh.
5. **Apply the theme** —
   `ui.ApplyTheme(a, sess.Settings().Theme)`
   ([main.go:32](main.go#L32)). This must happen **before** widget
   construction so initial layout uses the right palette.
6. **Create the window** —
   `w := a.NewWindow("Helena")` ([main.go:34](main.go#L34)),
   `w.SetIcon(icon)`, and resize to the session's remembered window size
   (default 1100x720) ([main.go:36-40](main.go#L36-L40)). `w.CenterOnScreen()`.
7. **Build the UI** —
   `mainUI := ui.NewMainUI(sess)` ([main.go:43](main.go#L43)) constructs
   every widget; `mainUI.SetWindow(w)` ([main.go:44](main.go#L44)) records
   the parent window and registers keyboard shortcuts. The order matters:
   `NewMainUI` runs first because shortcut registration needs the window's
   canvas (and the dialog actions need `m.win`).
8. **Install content** —
   `w.SetContent(fynetooltip.AddWindowToolTipLayer(mainUI.Root(), w.Canvas()))`
   ([main.go:48](main.go#L48)). The root is wrapped in fyne-tooltip's window
   layer so the icon-only toolbar/sidebar buttons can show hover tooltips.
9. **Wire startup diagnostic** —
   `a.Lifecycle().SetOnStarted(mainUI.SurfaceLoadErrors)` shows a
   non-transient dialog listing any collections that failed to load, deferred
   to OnStarted so it renders against an already-shown window (#108). A no-op
   when every collection loaded.
10. **Wire shutdown** —
   `a.Lifecycle().SetOnStopped(...)` records the final window size into the
   session so the next launch restores it.
11. **Run** —
    `w.ShowAndRun()` shows the window and blocks on the Fyne event loop until
    the app quits.
