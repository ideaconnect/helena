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

	res := m.sess.ResolverForNode(m.currentRequestID, &req)
	settings := m.sess.Settings()

	// mkTab builds one read-only snippet tab with its own Copy button, so adding
	// a codegen target is a single line (#95).
	mkTab := func(label, copyMsg string, gen func() (string, error)) *container.TabItem {
		entry := newSnippetEntry(gen())
		copyBtn := widget.NewButton("Copy", func() {
			fyne.CurrentApp().Clipboard().SetContent(entry.Text)
			m.Status.SetText("Copied " + copyMsg)
		})
		return container.NewTabItem(label, container.NewBorder(nil, copyBtn, nil, nil, container.NewScroll(entry)))
	}

	tabs := container.NewAppTabs(
		mkTab("cURL", "cURL command", func() (string, error) { return exporter.ToCurl(req, res, settings) }),
		mkTab("wget", "wget command", func() (string, error) { return exporter.ToWget(req, res, settings) }),
		mkTab("JavaScript", "fetch snippet", func() (string, error) { return exporter.ToFetch(req, res, settings) }),
		mkTab("Python", "Python snippet", func() (string, error) { return exporter.ToPython(req, res, settings) }),
		mkTab("Go", "Go snippet", func() (string, error) { return exporter.ToGo(req, res, settings) }),
	)

	// tabsTheme keeps the 4pt active-tab indicator here too (Fyne 2.8 sizes it
	// from SizeNameSeparatorThickness, which the app theme pins to 1). This scope
	// is transient — it lives only as long as the dialog.
	d := dialog.NewCustom("Export request", "Close", container.NewThemeOverride(tabs, tabsTheme{}), m.win)
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
