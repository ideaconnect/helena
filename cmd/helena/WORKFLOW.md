# cmd/helena — workflows

## Startup sequence

The entire entrypoint is a single linear flow. See STRUCTURE.md for a
detailed line-by-line map; this is the high-level view callers usually need.

```
main()
  -> app.NewWithID(...)
  -> assets.AppIcon -> fyne.NewStaticResource -> a.SetIcon
  -> config.DefaultPath()                     (fail-soft: empty path)
  -> session.New(cfgPath)                     (fail-soft: retry with "")
  -> ui.ApplyTheme(a, sess.Settings().Theme)  (must precede widgets)
  -> w := a.NewWindow("Helena")
  -> w.SetIcon(...)
  -> w.Resize(sess.WindowSize() OR 1100x720)
  -> w.CenterOnScreen()
  -> mainUI := ui.NewMainUI(sess)             (builds widgets, m.win is nil)
  -> mainUI.SetWindow(w)                      (records m.win, installs shortcuts)
  -> w.SetContent(mainUI.Root())
  -> a.Lifecycle().SetOnStopped(...)          (persist window size on quit)
  -> w.ShowAndRun()                           (blocks until quit)
```

Key invariants:

- **Theme before widgets.** `ApplyTheme` switches Fyne's active theme before
  `NewMainUI` so the first paint already uses the user's chosen palette.
- **`NewMainUI` runs before `SetWindow`.** This is intentional. `NewMainUI`
  cannot take a window because the UI tests in `internal/ui` build the UI
  in headless test environments where a Fyne window does not yet exist.
  `SetWindow` (called only from `cmd/helena`) is the dependency-injection
  point for the dialog parent + keyboard shortcuts.
- **Fail-soft persistence.** Config-path resolution and session load both
  fall back to in-memory (`""`) rather than aborting the launch. Helena
  always brings up a usable window.
- **Window size persistence happens on app stop**, via
  `a.Lifecycle().SetOnStopped`, which fires before `ShowAndRun` returns.
- **Blocking call.** `ShowAndRun` blocks; `main` returns only when the user
  closes the window or quits the app.
