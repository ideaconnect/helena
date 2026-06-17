package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/session"
)

// walkObjects visits o and all of its descendants (container children and
// widget renderer objects).
func walkObjects(o fyne.CanvasObject, fn func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	fn(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, c := range v.Objects {
			walkObjects(c, fn)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			walkObjects(c, fn)
		}
	}
}

// TestWindowTitleReflectsActiveWorkspace pins #124: the window title ends with
// the active workspace name and updates when the workspace switches.
func TestWindowTitleReflectsActiveWorkspace(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	active := s.WorkspaceNames()[s.ActiveIndex()] // "Default"

	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	if got := w.Title(); !strings.HasSuffix(got, "— "+active) {
		t.Errorf("initial title %q should end with active workspace %q", got, active)
	}

	if err := s.AddWorkspace("WS2"); err != nil {
		t.Fatal(err)
	}
	m.refreshWorkspaceDropdown()
	m.onWorkspaceChanged("WS2") // user picks the new workspace

	if got := w.Title(); !strings.HasSuffix(got, "— WS2") {
		t.Errorf("after switch, title %q should end with \"— WS2\"", got)
	}
}

// TestWorkspacesDialogGatesRenameDelete pins #130 (+ #122 icon buttons): the
// Workspaces dialog's Rename/Delete buttons are disabled with no selection and
// enable once a row is selected. The dialog's three action buttons are the only
// tooltip (ttwidget) buttons in the overlay.
func TestWorkspacesDialogGatesRenameDelete(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(700, 500))
	defer w.Close()
	m.SetWindow(w)

	m.editWorkspaces()

	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("workspaces dialog did not open")
	}
	var buttons []*ttwidget.Button
	var list *widget.List
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ttwidget.Button:
			buttons = append(buttons, v)
		case *widget.List:
			if list == nil {
				list = v
			}
		}
	})
	if len(buttons) != 3 {
		t.Fatalf("expected 3 action buttons (Add/Rename/Delete), got %d", len(buttons))
	}
	if list == nil {
		t.Fatal("workspace list not found in dialog")
	}

	enabled := func() int {
		n := 0
		for _, b := range buttons {
			if !b.Disabled() {
				n++
			}
		}
		return n
	}
	if got := enabled(); got != 1 { // only Add enabled with nothing selected
		t.Errorf("with no selection, %d of 3 buttons enabled; want 1 (Add only)", got)
	}

	list.Select(0)
	if got := enabled(); got != 3 { // selecting a row enables Rename + Delete
		t.Errorf("after selecting a workspace, %d of 3 buttons enabled; want 3", got)
	}

	list.Unselect(0)
	if got := enabled(); got != 1 {
		t.Errorf("after unselecting, %d of 3 buttons enabled; want 1", got)
	}
}
