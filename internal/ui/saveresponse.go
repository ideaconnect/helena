package ui

import (
	"errors"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// currentResponseBytes returns the raw bytes of the active tab's last response,
// byte-for-byte. The returned slice is the tab's cached buffer (also retained
// by the response viewer) — callers must only read it, never mutate. Returns
// false when there is no successful response to save (no tab, no response yet,
// or an error placeholder).
func (m *MainUI) currentResponseBytes() ([]byte, bool) {
	t := m.activeTab()
	if t == nil || t.resp == nil || t.resp.isError {
		return nil, false
	}
	return t.resp.rawBody, true
}

// writeResponseTo writes the active response's raw bytes to w, byte-for-byte.
// Factored out from saveResponseToFile so the exact-bytes write is testable
// without the file dialog.
func (m *MainUI) writeResponseTo(w io.Writer) (int, error) {
	b, ok := m.currentResponseBytes()
	if !ok {
		return 0, errors.New("no response to save")
	}
	return w.Write(b)
}

// saveResponseToFile prompts for a destination and writes the exact response
// bytes there — for large or binary bodies that the copy actions can't handle
// (#66).
func (m *MainUI) saveResponseToFile() {
	if m.win == nil {
		return
	}
	if _, ok := m.currentResponseBytes(); !ok {
		m.Status.SetText("No response to save")
		return
	}
	save := dialog.NewFileSave(func(wc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if wc == nil {
			return // cancelled
		}
		defer func() { _ = wc.Close() }()
		if _, err := m.writeResponseTo(wc); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Status.SetText("Saved response to " + wc.URI().Name())
	}, m.win)
	m.showFileDialog(save)
}
