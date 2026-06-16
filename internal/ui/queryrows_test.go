package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

func newParamsUI(t *testing.T) *MainUI {
	t.Helper()
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	test.NewWindow(m.Root())
	return m
}

// TestQueryRowsReusedOnValueEdit verifies that editing a query-param value via
// the URL field (param count unchanged) updates the existing row widgets in
// place rather than tearing them all down and rebuilding — the row handles are
// the same pointers afterward, and the value is reflected (#53).
func TestQueryRowsReusedOnValueEdit(t *testing.T) {
	m := newParamsUI(t)
	m.currentRequest = &model.Request{
		Method: model.GET,
		URL:    "https://x/api",
		Params: []model.KeyValue{
			{Enabled: true, Key: "a", Value: "1"},
			{Enabled: true, Key: "b", Value: "2"},
		},
	}
	m.rebuildParamsRows()
	if len(m.paramRows) != 2 {
		t.Fatalf("setup: %d rows, want 2", len(m.paramRows))
	}
	row0, row1 := m.paramRows[0], m.paramRows[1]

	// Simulate the URL field changing b's value (same param count).
	m.applyURLEdit("https://x/api?a=1&b=9")

	if len(m.paramRows) != 2 {
		t.Fatalf("after value edit: %d rows, want 2", len(m.paramRows))
	}
	if m.paramRows[0] != row0 || m.paramRows[1] != row1 {
		t.Error("row widgets were rebuilt on a value-only edit (expected in-place reuse)")
	}
	if m.paramRows[1].val.Text != "9" {
		t.Errorf("row value not synced in place: got %q, want 9", m.paramRows[1].val.Text)
	}
	if m.currentRequest.Params[1].Value != "9" {
		t.Errorf("backing param not updated: %+v", m.currentRequest.Params[1])
	}
}

// TestQueryRowsRebuiltOnCountChange verifies that adding a param via the URL
// (count change) falls back to a full rebuild, so two-way sync still reflects
// the new row.
func TestQueryRowsRebuiltOnCountChange(t *testing.T) {
	m := newParamsUI(t)
	m.currentRequest = &model.Request{
		Method: model.GET,
		URL:    "https://x/api",
		Params: []model.KeyValue{{Enabled: true, Key: "a", Value: "1"}},
	}
	m.rebuildParamsRows()
	if len(m.paramRows) != 1 {
		t.Fatalf("setup: %d rows, want 1", len(m.paramRows))
	}

	m.applyURLEdit("https://x/api?a=1&b=2")

	if len(m.paramRows) != 2 {
		t.Fatalf("after adding a param: %d rows, want 2", len(m.paramRows))
	}
	if m.paramRows[1].key.Text != "b" || m.paramRows[1].val.Text != "2" {
		t.Errorf("new row not reflected: key=%q val=%q", m.paramRows[1].key.Text, m.paramRows[1].val.Text)
	}
}
