package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// shortcutEntry is a widget.Entry that also checks the app's own keyboard
// shortcuts (registerShortcuts, shortcuts.go) before falling back to Entry's
// built-in Cut/Copy/Paste/Undo/Redo/SelectAll handling. Needed because Fyne
// routes a detected shortcut to the focused widget's own TypedShortcut instead
// of the window canvas's Canvas.AddShortcut bindings whenever the focused
// widget implements fyne.Shortcutable — which widget.Entry does — so without
// this, every app-wide shortcut goes silently dead the moment any entry has
// focus (which, in an editor-heavy app, is most of the time).
type shortcutEntry struct {
	widget.Entry
	m *MainUI
}

// newShortcutEntry returns a single-line shortcutEntry, a drop-in replacement
// for widget.NewEntry() at construction sites whose field commonly holds
// keyboard focus during normal use.
func (m *MainUI) newShortcutEntry() *shortcutEntry {
	e := &shortcutEntry{m: m}
	e.ExtendBaseWidget(e)
	return e
}

// newShortcutMultiLineEntry returns a multi-line shortcutEntry, a drop-in
// replacement for widget.NewMultiLineEntry().
func (m *MainUI) newShortcutMultiLineEntry() *shortcutEntry {
	e := &shortcutEntry{m: m}
	e.MultiLine = true
	e.ExtendBaseWidget(e)
	return e
}

// TypedShortcut checks the app-wide shortcut table first; anything it doesn't
// recognize (Cut/Copy/Paste/Undo/Redo/SelectAll, or a shortcut fired before
// SetWindow has registered m.shortcuts) falls through to Entry's own handling.
//
// Undo/Redo already reach Entry: Fyne's driver maps Ctrl+Z / Ctrl+Y to the
// standard fyne.ShortcutUndo / fyne.ShortcutRedo (not a CustomShortcut), so
// those bypass the app-table check below and land on Entry.TypedShortcut. The
// one gap is the Ctrl/Cmd+Shift+Z redo chord (common on Linux/macOS), which the
// driver reports as a CustomShortcut and Fyne otherwise ignores — map it to the
// Entry's own redo so both redo chords work.
func (e *shortcutEntry) TypedShortcut(s fyne.Shortcut) {
	if cs, ok := s.(*desktop.CustomShortcut); ok {
		if isRedoChord(cs) {
			e.Entry.TypedShortcut(&fyne.ShortcutRedo{})
			return
		}
		if do := e.m.shortcutFor(cs); do != nil {
			do()
			return
		}
	}
	e.Entry.TypedShortcut(s)
}

// isRedoChord reports whether cs is Ctrl/Cmd+Shift+Z — the alternate redo chord
// Fyne's driver leaves as a CustomShortcut (it only wires Ctrl+Y to
// fyne.ShortcutRedo). The modifier is built the same way registerShortcuts /
// shortcutFor build theirs, so it matches on every platform.
func isRedoChord(cs *desktop.CustomShortcut) bool {
	return cs.KeyName == fyne.KeyZ &&
		cs.Modifier == fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift
}
