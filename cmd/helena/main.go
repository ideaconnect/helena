// Command helena is the entry point for the Helena API client.
package main

import (
	"fmt"
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"github.com/idct/helena/assets"
	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/ui"
)

// Build metadata, injected at release time via the linker
// (-ldflags "-X main.version=... -X main.commit=... -X main.date=...").
// Defaults keep a local `go build` self-describing as a dev build.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// appID is the Fyne application ID. It MUST match the `ID` in FyneApp.toml so
// the native `fyne package` tooling and the running app agree on identity; a
// test guards against drift.
const appID = "tech.idct.helena"

// versionString renders the build metadata for `helena --version`. The commit
// is trimmed to a short hash for readability; empty fields are omitted.
func versionString(version, commit, date string) string {
	s := "helena " + version
	if commit != "" {
		short := commit
		if len(short) > 12 {
			short = short[:12]
		}
		s += " (" + short + ")"
	}
	if date != "" {
		s += " built " + date
	}
	return s
}

// windowTitle is the app window title, suffixed with the version for a
// non-dev (released) build so the running build is identifiable at a glance.
func windowTitle(version string) string {
	if version == "dev" || version == "" {
		return "Helena"
	}
	return "Helena " + version
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(versionString(version, commit, date))
		return
	}

	// Process-level safety net: log a panic during setup before the process
	// exits, so a crash leaves a breadcrumb for bug reports (#48). UI event
	// handlers are additionally guarded inside the ui package.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("helena: fatal panic: %v", r)
			panic(r) // re-panic so the runtime still prints the stack + exits non-zero
		}
	}()

	a := app.NewWithID(appID)
	icon := fyne.NewStaticResource("app_icon.png", assets.AppIcon)
	a.SetIcon(icon)

	cfgPath, err := config.DefaultPath()
	if err != nil {
		log.Printf("helena: config path unavailable, running without persistence: %v", err)
		cfgPath = ""
	}
	sess, err := session.New(cfgPath)
	if err != nil {
		log.Printf("helena: could not load session, starting fresh: %v", err)
		sess, _ = session.New("")
	}

	ui.ApplyTheme(a, sess.Settings().Theme)

	w := a.NewWindow(windowTitle(version))
	w.SetIcon(icon)
	if ww, wh := sess.WindowSize(); ww > 0 && wh > 0 {
		w.Resize(fyne.NewSize(float32(ww), float32(wh)))
	} else {
		w.Resize(fyne.NewSize(1100, 720))
	}
	w.CenterOnScreen()

	mainUI := ui.NewMainUI(sess)
	mainUI.SetWindow(w)
	// Wrap in a tooltip layer (fyne-tooltip) so the icon-only toolbar buttons can
	// show hover tooltips — Fyne core has no tooltip support.
	w.SetContent(fynetooltip.AddWindowToolTipLayer(mainUI.Root(), w.Canvas()))

	// Surface any collections that failed to load so they don't silently
	// vanish from the sidebar (#108). Deferred to OnStarted so the dialog
	// renders against an already-shown window.
	a.Lifecycle().SetOnStarted(mainUI.SurfaceLoadErrors)

	a.Lifecycle().SetOnStopped(func() {
		size := w.Canvas().Size()
		sess.SetWindowSize(int(size.Width), int(size.Height))
	})

	w.ShowAndRun()
}
