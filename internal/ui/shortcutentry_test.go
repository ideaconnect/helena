package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestShortcutEntryDispatchesRegisteredShortcut reproduces the core bug: Fyne
// routes a detected shortcut to the FOCUSED widget's own TypedShortcut instead
// of the canvas's Canvas.AddShortcut handlers whenever that widget implements
// fyne.Shortcutable (which widget.Entry does). Before shortcutEntry existed,
// an app-wide binding like Ctrl+S never reached m.shortcuts while any entry
// had focus. Bypasses registerShortcuts (which wires real, side-effecting
// actions) and injects a fake one directly, isolating the dispatch path.
func TestShortcutEntryDispatchesRegisteredShortcut(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	fired := 0
	m.shortcuts = []shortcutSpec{
		{fyne.KeyS, 0, "S", "test save", func() { fired++ }},
	}

	e := m.newShortcutEntry()
	e.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault})

	if fired != 1 {
		t.Errorf("registered shortcut fired %d times while entry had focus, want 1", fired)
	}
}

// TestShortcutEntryFallsBackToNativeShortcut verifies shortcutEntry does not
// break Entry's own built-in shortcuts (Copy, SelectAll, etc.) for anything
// that isn't in m.shortcuts.
func TestShortcutEntryFallsBackToNativeShortcut(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	m.shortcuts = nil // nothing registered

	e := m.newShortcutEntry()
	e.SetText("hello world")
	e.TypedShortcut(&fyne.ShortcutSelectAll{})

	if got := e.SelectedText(); got != "hello world" {
		t.Errorf("native SelectAll did not run through the fallback path; SelectedText() = %q", got)
	}
}

// TestShortcutForMatchesRegisteredBinding exercises shortcutFor's lookup
// directly, including a Shift-modified binding (New collection), and confirms
// an unregistered key/modifier combination misses cleanly.
func TestShortcutForMatchesRegisteredBinding(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	var plainFired, shiftFired int
	m.shortcuts = []shortcutSpec{
		{fyne.KeyN, 0, "N", "new request", func() { plainFired++ }},
		{fyne.KeyN, fyne.KeyModifierShift, "Shift+N", "new collection", func() { shiftFired++ }},
	}

	if do := m.shortcutFor(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierShortcutDefault}); do == nil {
		t.Error("plain Mod+N did not match")
	} else {
		do()
	}
	if do := m.shortcutFor(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}); do == nil {
		t.Error("Mod+Shift+N did not match")
	} else {
		do()
	}
	if plainFired != 1 || shiftFired != 1 {
		t.Errorf("plainFired=%d shiftFired=%d, want 1 and 1 (the two bindings must not cross-fire)", plainFired, shiftFired)
	}

	if do := m.shortcutFor(&desktop.CustomShortcut{KeyName: fyne.KeyX, Modifier: fyne.KeyModifierShortcutDefault}); do != nil {
		t.Error("unregistered shortcut unexpectedly matched")
	}
}

// TestRegisterShortcutsEndToEndThroughFocusedWidgets is the full regression
// test for the reported bug ("none of the keyboard shortcuts seem to work"):
// after a real SetWindow/registerShortcuts, firing a registered shortcut
// directly on a focused shortcutEntry (m.URL) and a focused PrettyView (m.pv)
// must run the real action, not silently do nothing.
func TestRegisterShortcutsEndToEndThroughFocusedWidgets(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	undoZ := &desktop.CustomShortcut{KeyName: fyne.KeyZ, Modifier: fyne.KeyModifierShortcutDefault}

	m.Status.Text = ""
	m.URL.TypedShortcut(undoZ)
	if m.Status.Text == "" {
		t.Error("Ctrl+Z through the focused URL entry did not reach actionUndoDelete")
	}

	m.Status.Text = ""
	m.pv.TypedShortcut(undoZ)
	if m.Status.Text == "" {
		t.Error("Ctrl+Z through the focused response PrettyView did not reach actionUndoDelete")
	}
}
