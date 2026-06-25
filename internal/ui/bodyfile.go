package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// buildBodyFilePanel builds the BodyFile editor (#24): a chosen-file label with
// Choose / Clear buttons and a Content-Type entry. The request sends the exact
// bytes of the chosen file; an empty Content-Type defaults to
// application/octet-stream at send time. Hidden until the Body type is "file".
func (m *MainUI) buildBodyFilePanel() fyne.CanvasObject {
	m.bodyFilePathLabel = widget.NewLabel("No file chosen")
	m.bodyFilePathLabel.Truncation = fyne.TextTruncateEllipsis
	chooseBtn := widget.NewButtonWithIcon("Choose file…", themedIcon("folder-open"), m.chooseBodyFile)
	clearBtn := widget.NewButton("Clear", m.clearBodyFile)

	m.bodyFileContentType = widget.NewEntry()
	m.bodyFileContentType.SetPlaceHolder("application/octet-stream")
	m.bodyFileContentType.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Body.ContentType = s
		}
	}

	fileRow := container.NewBorder(nil, nil, chooseBtn, clearBtn, m.bodyFilePathLabel)
	ctRow := container.NewBorder(nil, nil, widget.NewLabel("Content-Type:"), nil, m.bodyFileContentType)
	hint := widget.NewLabel("The exact bytes of the chosen file are sent as the request body.")
	hint.Wrapping = fyne.TextWrapWord
	m.bodyFilePanel = container.NewVBox(fileRow, ctRow, hint)
	m.bodyFilePanel.Hide()
	return m.bodyFilePanel
}

// chooseBodyFile opens a file picker and records the selection on the current
// request's Body.FilePath (#24).
func (m *MainUI) chooseBodyFile() {
	if m.win == nil || m.currentRequest == nil {
		return
	}
	dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		defer func() { _ = rc.Close() }()
		path := rc.URI().Path()
		if m.currentRequest != nil {
			m.currentRequest.Body.FilePath = path
		}
		m.bodyFilePathLabel.SetText(path)
	}, m.win)
}

// clearBodyFile drops the chosen file from the current request.
func (m *MainUI) clearBodyFile() {
	if m.currentRequest != nil {
		m.currentRequest.Body.FilePath = ""
	}
	m.bodyFilePathLabel.SetText("No file chosen")
}

// loadBodyFilePanel populates the file-panel widgets from a request body; called
// under m.loading from loadRequest so the entry's OnChanged doesn't write back.
func (m *MainUI) loadBodyFilePanel(b model.Body) {
	if m.bodyFilePathLabel == nil {
		return
	}
	if b.FilePath != "" {
		m.bodyFilePathLabel.SetText(b.FilePath)
	} else {
		m.bodyFilePathLabel.SetText("No file chosen")
	}
	m.bodyFileContentType.SetText(b.ContentType)
}
