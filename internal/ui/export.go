package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/exporter"
	"github.com/idct/helena/internal/model"
)

// actionExport renders the active edited request (or the bare method/URL if no
// tree item is selected) as cURL and wget commands in a tabbed dialog with
// Copy buttons.
func (m *MainUI) actionExport() {
	if m.win == nil {
		return
	}
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}
	var req model.Request
	if m.currentRequest != nil {
		req = *m.currentRequest
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected()), URL: m.URL.Text}
	}

	res := m.sess.Resolver()
	settings := m.sess.Settings()

	curlEntry := newSnippetEntry(exporter.ToCurl(req, res, settings))
	wgetEntry := newSnippetEntry(exporter.ToWget(req, res, settings))

	copyCurl := widget.NewButton("Copy", func() {
		fyne.CurrentApp().Clipboard().SetContent(curlEntry.Text)
		m.Status.SetText("Copied cURL command")
	})
	copyWget := widget.NewButton("Copy", func() {
		fyne.CurrentApp().Clipboard().SetContent(wgetEntry.Text)
		m.Status.SetText("Copied wget command")
	})

	curlPane := container.NewBorder(nil, copyCurl, nil, nil, container.NewScroll(curlEntry))
	wgetPane := container.NewBorder(nil, copyWget, nil, nil, container.NewScroll(wgetEntry))

	tabs := container.NewAppTabs(
		container.NewTabItem("cURL", curlPane),
		container.NewTabItem("wget", wgetPane),
	)

	d := dialog.NewCustom("Export request", "Close", tabs, m.win)
	d.Resize(fyne.NewSize(680, 420))
	d.Show()
}

// newSnippetEntry builds a read-only multi-line entry showing either the
// exporter's output or an error comment, never panicking on a nil exporter.
// It is disabled so the generated command can't be edited and an accidental
// edit copied — the Copy buttons read the entry's Text directly, so copy still
// works on the unmodified snippet.
func newSnippetEntry(text string, err error) *widget.Entry {
	e := widget.NewMultiLineEntry()
	e.Wrapping = fyne.TextWrapOff
	if err != nil {
		e.SetText("# error: " + err.Error())
	} else {
		e.SetText(text)
	}
	e.Disable()
	return e
}
