// Command helena is the entry point for the Helena API client.
package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/idct/helena/assets"
	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/ui"
)

func main() {
	a := app.NewWithID("tech.idct.helena")
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

	w := a.NewWindow("Helena")
	w.SetIcon(icon)
	if ww, wh := sess.WindowSize(); ww > 0 && wh > 0 {
		w.Resize(fyne.NewSize(float32(ww), float32(wh)))
	} else {
		w.Resize(fyne.NewSize(1100, 720))
	}
	w.CenterOnScreen()

	mainUI := ui.NewMainUI(sess)
	mainUI.SetWindow(w)
	w.SetContent(mainUI.Root())

	a.Lifecycle().SetOnStopped(func() {
		size := w.Canvas().Size()
		sess.SetWindowSize(int(size.Width), int(size.Height))
	})

	w.ShowAndRun()
}
