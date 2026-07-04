package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/runner"
	"github.com/idct/helena/internal/session"
)

// TestRunCollectionNoCollection verifies the run action no-ops with a hint when
// no collection is open (#89).
func TestRunCollectionNoCollection(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	m.actionRunCollection()
	if got := m.Status.Text; !strings.Contains(got, "Open a collection") {
		t.Errorf("status = %q, want an 'open a collection' hint", got)
	}
}

// TestShowRunReportSummary verifies the report dialog sets a pass/fail summary
// status from a synthetic Report.
func TestShowRunReportSummary(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(900, 600))
	defer w.Close()
	m.SetWindow(w)

	rep := runner.Report{Results: []runner.RequestResult{
		{Path: "Health", Method: "GET", StatusCode: 200, Checks: []runner.Check{{Name: "status", Passed: true}}},
		{Path: "Broken", Method: "GET", StatusCode: 500, Checks: []runner.Check{{Name: "status", Passed: false, Error: "bad"}}},
	}}
	m.showRunReport(rep, "collection")
	if got := m.Status.Text; !strings.Contains(got, "FAILED") || !strings.Contains(got, "1 failed") {
		t.Errorf("status = %q, want a FAILED summary with 1 failed", got)
	}
}
