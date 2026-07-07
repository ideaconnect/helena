package ui

import (
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ShowSaving pops a small, buttonless modal with a spinner and "Saving…" text.
// It is shown on the way out (window close) so a close that has to flush state —
// and, on some platforms, tear down the GL context — shows feedback instead of a
// frozen window. It is never explicitly dismissed: the process exits from under
// it (see cmd/helena's close intercept). No-op when there is no window (headless
// / tests that skip SetWindow).
func (m *MainUI) ShowSaving() {
	if m.win == nil {
		return
	}
	spinner := widget.NewActivity()
	spinner.Start()
	content := container.NewHBox(spinner, widget.NewLabel("Saving…"))
	dialog.NewCustomWithoutButtons("", content, m.win).Show()
}
