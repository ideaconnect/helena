package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/history"
)

// showHistory opens the send-history viewer (#65): a newest-first list of past
// sends with Restore (open the request in a scratch tab), Resend (open + send),
// and Clear. Reached from Help → History. Secrets were scrubbed at record time,
// so a restored request keeps everything but the credential — a resend
// re-resolves it from the active environment.
func (m *MainUI) showHistory() {
	if m.win == nil {
		return
	}
	entries := m.sess.History().Entries()

	list := widget.NewList(
		func() int { return len(entries) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(historySummary(entries[i]))
		},
	)

	selectedIdx := -1
	var restoreBtn, resendBtn *ttwidget.Button
	selectionChanged := func() {
		enableButton(restoreBtn, selectedIdx >= 0)
		enableButton(resendBtn, selectedIdx >= 0)
	}
	list.OnSelected = func(i widget.ListItemID) { selectedIdx = i; selectionChanged() }
	list.OnUnselected = func(widget.ListItemID) { selectedIdx = -1; selectionChanged() }

	var d dialog.Dialog
	restoreBtn = tipButton("pen-to-square", "Restore in a new tab", func() {
		if selectedIdx < 0 {
			return
		}
		req := entries[selectedIdx].Request
		d.Hide()
		m.openScratchWith(req)
	})
	resendBtn = tipButton("play", "Resend", func() {
		if selectedIdx < 0 {
			return
		}
		req := entries[selectedIdx].Request
		d.Hide()
		m.openScratchWith(req)
		m.send()
	})
	clearBtn := tipButton("trash-can", "Clear history", func() {
		if len(entries) == 0 {
			return
		}
		dialog.ShowConfirm("Clear history?", "Remove every recorded send from the history?",
			func(yes bool) {
				if yes {
					m.sess.History().Clear()
					d.Hide()
				}
			}, m.win)
	})
	selectionChanged() // start with Restore/Resend disabled (no selection)

	toolbar := container.NewHBox(restoreBtn, resendBtn, clearBtn)
	var content fyne.CanvasObject = list
	if len(entries) == 0 {
		content = widget.NewLabel("No history yet — send a request and it will appear here.")
	}
	body := container.NewBorder(toolbar, nil, nil, nil, content)
	d = dialog.NewCustom("History", "Close", body, m.win)
	d.Resize(fyne.NewSize(680, 460))
	d.Show()
}

// historySummary renders one history row: method, status, URL, and a relative
// timestamp.
func historySummary(e history.Entry) string {
	status := "—"
	switch {
	case e.Status != 0:
		status = fmt.Sprintf("%d", e.Status)
	case e.Err != "":
		status = "ERR"
	}
	return fmt.Sprintf("%-6s %-4s %s   · %s", e.Method, status, e.URL, humanAgo(e.Time))
}

// humanAgo renders a compact relative time ("just now" / "3m ago" / "2h ago"),
// falling back to a date-time for entries older than a day.
func humanAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}
