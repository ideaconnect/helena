# cmd/helena — workflows

## Startup sequence

The entire entrypoint is a single linear flow. See STRUCTURE.md for a
detailed line-by-line map; this is the high-level view callers usually need.

```
main()
  -> flag.Parse()  (--version, --verbose, --log-file)
  -> if --version: print versionString() and return  (#26, before any UI)
  -> logging.Configure(verbose, log-file OR $HELENA_LOG)  (#49) -> log "helena starting"
  -> defer recover()+logging.L().Error(stack)  (process-level safety net for setup panics, #48/#49; re-panics so the runtime still exits non-zero)
  -> app.NewWithID(...)
  -> assets.AppIcon -> fyne.NewStaticResource -> a.SetIcon
  -> config.DefaultPath()                     (fail-soft: empty path)
  -> session.New(cfgPath)                     (fail-soft: retry with "")
  -> ui.ApplyTheme(a, sess.Settings().Theme)  (must precede widgets)
  -> w := a.NewWindow(windowTitle(version))   (suffixes the version for a released build)
  -> w.SetIcon(...)
  -> w.Resize(sess.WindowSize() OR 1100x720)
  -> w.CenterOnScreen()
  -> mainUI := ui.NewMainUI(sess)             (builds widgets, m.win is nil)
  -> mainUI.SetWindow(w)                      (records m.win, installs shortcuts)
  -> w.SetContent(fynetooltip.AddWindowToolTipLayer(mainUI.Root(), w.Canvas()))  (tooltip layer for icon-only buttons)
  -> a.Lifecycle().SetOnStarted(mainUI.SurfaceLoadErrors)  (diagnostic dialog for collections that failed to load, #108)
  -> a.Lifecycle().SetOnStopped(saveWindowState)  (persist window size on app.Quit paths)
  -> w.SetCloseIntercept(...)                 (window close button: persist + os.Exit, skipping the slow GL teardown)
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
- **Load-error surfacing happens on app start**, via
  `a.Lifecycle().SetOnStarted(mainUI.SurfaceLoadErrors)`, so the diagnostic
  dialog renders against an already-shown window rather than during
  construction. It is a no-op when every collection loaded (#108).
- **Window size persistence happens on app stop**, via
  `a.Lifecycle().SetOnStopped` (the `app.Quit()` / menu / Ctrl-Q paths).
- **The window close button uses `SetCloseIntercept`.** Fyne tears the OpenGL
  context down (`glfw.Terminate`) on the UI thread *before* `OnStopped`, and on
  WSLg that teardown can stall for seconds, so the process appears to hang on
  close. The interceptor fires *before* teardown: it persists the window size
  and calls `os.Exit(0)`, skipping the stall (the OS reclaims the GL resources).
  Safe because window size is the only shutdown-time state — collections and
  config are written on edit.
- **Blocking call.** `ShowAndRun` blocks; on `app.Quit()` paths `main` returns
  when the event loop ends, while the close button exits from the interceptor.
