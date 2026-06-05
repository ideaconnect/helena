package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// TestTreeRowRequestVsBranch verifies the display widget shows a brand-colored
// method chip for requests and hides it for branches, and that the name
// truncates rather than overflowing.
func TestTreeRowRequestVsBranch(t *testing.T) {
	r := newTreeRow(nil, nil, nil)
	if r.name.Truncation != fyne.TextTruncateEllipsis {
		t.Errorf("name truncation = %v; want ellipsis", r.name.Truncation)
	}

	r.setRequest("0/r0", "GET", "Get widgets")
	if r.id != "0/r0" {
		t.Errorf("id = %q; want 0/r0", r.id)
	}
	if !r.method.Visible() {
		t.Error("request row should show its method chip")
	}
	if r.method.Text != "GET" {
		t.Errorf("method chip = %q; want GET", r.method.Text)
	}
	if r.method.Color != methodColor("GET") {
		t.Error("method chip not tinted to the GET brand colour")
	}
	if r.name.Text != "Get widgets" {
		t.Errorf("name = %q", r.name.Text)
	}

	r.setBranch("0/f0", "drs")
	if r.id != "0/f0" {
		t.Errorf("id = %q; want 0/f0", r.id)
	}
	if r.method.Visible() {
		t.Error("branch row should hide the method chip")
	}
	if r.name.Text != "drs" {
		t.Errorf("branch label = %q; want drs", r.name.Text)
	}
}

// TestTreeRowDragCallbacks verifies the row forwards drag events with its id.
func TestTreeRowDragCallbacks(t *testing.T) {
	var draggedID, endedID string
	r := newTreeRow(
		func(id string, _ *fyne.DragEvent) { draggedID = id },
		func(id string) { endedID = id },
		nil,
	)
	r.setRequest("0/r3", "POST", "Create")
	r.Dragged(&fyne.DragEvent{})
	r.DragEnd()
	if draggedID != "0/r3" || endedID != "0/r3" {
		t.Errorf("drag callbacks got (%q,%q); want 0/r3 both", draggedID, endedID)
	}
}

// TestTreeRowCursor verifies the row reports a grab cursor only while a drag is
// in flight.
func TestTreeRowCursor(t *testing.T) {
	dragging := false
	r := newTreeRow(nil, nil, func() bool { return dragging })
	if r.Cursor() != desktop.DefaultCursor {
		t.Errorf("idle cursor = %v; want default", r.Cursor())
	}
	dragging = true
	if r.Cursor() != desktop.PointerCursor {
		t.Errorf("dragging cursor = %v; want pointer", r.Cursor())
	}
	// nil provider must not panic.
	if newTreeRow(nil, nil, nil).Cursor() != desktop.DefaultCursor {
		t.Error("nil dragging provider should yield default cursor")
	}
}
