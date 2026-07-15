package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
)

// These tests pin that undo/redo reaches Helena's text surfaces. Fyne's driver
// maps Ctrl+Z / Ctrl+Y to the standard fyne.ShortcutUndo / fyne.ShortcutRedo and
// delivers them to the focused widget's TypedShortcut, so the assertions drive
// those shortcuts directly — the same objects the driver would hand over. They
// guard against a future shortcutEntry / editor change silently swallowing them.

// TestShortcutEntryUndoRedo verifies a plain field (the params/auth/headers row
// type) undoes and redoes typed text, and that Ctrl+Shift+Z redoes too.
func TestShortcutEntryUndoRedo(t *testing.T) {
	m := newResponseUI(t)
	e := m.newShortcutEntry()
	w := test.NewWindow(e)
	defer w.Close()
	w.Canvas().Focus(e)

	test.Type(e, "hello")
	if e.Text != "hello" {
		t.Fatalf("after type = %q, want hello", e.Text)
	}
	e.TypedShortcut(&fyne.ShortcutUndo{})
	if e.Text != "" {
		t.Errorf("after Ctrl+Z = %q, want empty (undo)", e.Text)
	}
	e.TypedShortcut(&fyne.ShortcutRedo{})
	if e.Text != "hello" {
		t.Errorf("after Ctrl+Y = %q, want hello (redo)", e.Text)
	}
	// Ctrl+Shift+Z is the alternate redo chord (Fyne only wires Ctrl+Y).
	e.TypedShortcut(&fyne.ShortcutUndo{})
	e.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyZ, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if e.Text != "hello" {
		t.Errorf("after Ctrl+Shift+Z = %q, want hello (redo)", e.Text)
	}
}

// TestShortcutEntryUndoNotShadowedByAppShortcut pins that Ctrl+Z performs a text
// undo rather than the app's Ctrl+Z ("Undo last delete") action: the driver
// delivers ShortcutUndo (not a CustomShortcut), so shortcutEntry's app-table
// check must not intercept it.
func TestShortcutEntryUndoNotShadowedByAppShortcut(t *testing.T) {
	m := newResponseUI(t)
	fired := false
	m.shortcuts = []shortcutSpec{{fyne.KeyZ, 0, "Z", "Undo last delete", func() { fired = true }}}

	e := m.newShortcutEntry()
	w := test.NewWindow(e)
	defer w.Close()
	w.Canvas().Focus(e)
	test.Type(e, "abc")
	e.TypedShortcut(&fyne.ShortcutUndo{})

	if fired {
		t.Error("Ctrl+Z fired the app's tree-undo instead of a text undo")
	}
	if e.Text != "" {
		t.Errorf("text = %q, want empty (Ctrl+Z should undo the typed text)", e.Text)
	}
}

// TestURLBarUndoSyncsModel verifies the address bar undoes typed text and the
// undo flows back into currentRequest via the field's OnChanged.
func TestURLBarUndoSyncsModel(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Method: model.GET}
	m.loadRequest(req, "id-1")
	w := test.NewWindow(m.Root())
	defer w.Close()
	w.Canvas().Focus(m.URL)

	test.Type(m.URL, "https://x/a")
	m.URL.TypedShortcut(&fyne.ShortcutUndo{})
	if m.URL.Text == "https://x/a" {
		t.Errorf("URL field did not undo: %q", m.URL.Text)
	}
	if req.URL != m.URL.Text {
		t.Errorf("model URL %q out of sync with field %q after undo", req.URL, m.URL.Text)
	}
}

// TestEditorUndoRedo verifies the pretty-view body editor undoes and redoes
// typed text (the editor implements its own undo history).
func TestEditorUndoRedo(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Method: model.POST, Body: model.Body{Type: model.BodyText}}
	m.loadRequest(req, "id-1")
	w := test.NewWindow(m.Root())
	defer w.Close()
	w.Canvas().Focus(m.BodyContent)

	test.Type(m.BodyContent, "abc")
	if got := string(m.BodyContent.Source()); got != "abc" {
		t.Fatalf("after type = %q, want abc", got)
	}
	m.BodyContent.TypedShortcut(&fyne.ShortcutUndo{})
	if got := string(m.BodyContent.Source()); got != "" {
		t.Errorf("after Ctrl+Z = %q, want empty (editor undo)", got)
	}
	m.BodyContent.TypedShortcut(&fyne.ShortcutRedo{})
	if got := string(m.BodyContent.Source()); got != "abc" {
		t.Errorf("after Ctrl+Y = %q, want abc (editor redo)", got)
	}
}

// TestIsRedoChord pins the alternate-redo-chord matcher.
func TestIsRedoChord(t *testing.T) {
	yes := &desktop.CustomShortcut{KeyName: fyne.KeyZ, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	if !isRedoChord(yes) {
		t.Error("Ctrl+Shift+Z should be the redo chord")
	}
	for _, no := range []*desktop.CustomShortcut{
		{KeyName: fyne.KeyZ, Modifier: fyne.KeyModifierShortcutDefault},                         // plain Ctrl+Z (undo)
		{KeyName: fyne.KeyY, Modifier: fyne.KeyModifierShortcutDefault},                         // Ctrl+Y (redo)
		{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}, // Ctrl+Shift+S
	} {
		if isRedoChord(no) {
			t.Errorf("%v should not be the redo chord", no)
		}
	}
}
