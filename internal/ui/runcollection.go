package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/runner"
)

// actionRunCollection runs requests off the UI goroutine via the shared
// headless runner, then shows a per-request + aggregate pass/fail dialog. Scope
// follows the sidebar selection: a selected folder runs just that folder's
// subtree (#89), otherwise the whole active collection. No-op with a status hint
// when no collection is open; ignores a second click while a run is in flight.
func (m *MainUI) actionRunCollection() {
	if m.win == nil {
		return
	}
	if m.sess.ActiveCollection() < 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	if m.runningCollection {
		return
	}

	scopeID, scopeLabel := "", "collection"
	if m.isFolderSelected() {
		scopeID = m.lastSelectedNodeID
		scopeLabel = "folder " + m.sess.Tree().Label(scopeID)
		// Resolve vars / auth against the folder's own collection.
		if ci := m.sess.Tree().CollectionIndex(scopeID); ci >= 0 {
			m.sess.SetActiveCollection(ci)
		}
	}

	m.runningCollection = true
	m.Status.SetText("Running " + scopeLabel + "…")

	go func() {
		// runner.RunScope does network I/O and must not touch widgets; it reads
		// session snapshots and returns a plain Report. Marshal the result back
		// to the UI thread to render (invariant 4).
		rep := runner.RunScope(context.Background(), m.sess, scopeID)
		fyne.Do(func() {
			m.runningCollection = false
			m.showRunReport(rep, scopeLabel)
		})
	}()
}

// showRunReport renders a run Report as a scrollable dialog: an aggregate
// header line (naming the run scope) plus one row per request (ok/FAIL, status,
// path) with its failed checks indented beneath.
func (m *MainUI) showRunReport(rep runner.Report, scopeLabel string) {
	reqs, passed, failed := rep.Totals()
	verdict := "PASSED"
	if rep.Failed() {
		verdict = "FAILED"
	}
	summary := widget.NewLabelWithStyle(
		fmt.Sprintf("%s — %s: %d requests, %d checks passed, %d failed", verdict, scopeLabel, reqs, passed, failed),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	m.Status.SetText(fmt.Sprintf("Run %s: %d passed / %d failed", verdict, passed, failed))

	rows := container.NewVBox()
	for _, r := range rep.Results {
		mark := "ok"
		if !r.OK() {
			mark = "FAIL"
		}
		status := "—"
		if r.StatusCode != 0 {
			status = fmt.Sprintf("%d", r.StatusCode)
		}
		rows.Add(widget.NewLabel(fmt.Sprintf("%-4s  %-5s  %s", mark, status, r.Path)))
		if r.Err != "" {
			rows.Add(widget.NewLabel("        error: " + r.Err))
		}
		for _, c := range r.Checks {
			if c.Passed {
				continue // only surface failures per-row to keep the list scannable
			}
			line := "        FAIL  " + c.Name
			if c.Error != "" {
				line += " — " + c.Error
			}
			rows.Add(widget.NewLabel(line))
		}
	}

	body := container.NewBorder(summary, nil, nil, nil, container.NewVScroll(rows))
	d := dialog.NewCustom("Run results", "Close", body, m.win)
	d.Resize(fyne.NewSize(640, 460))
	d.Show()
}
