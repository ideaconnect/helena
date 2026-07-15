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
func (e *shortcutEntry) TypedShortcut(s fyne.Shortcut) {
	if cs, ok := s.(*desktop.CustomShortcut); ok {
		if do := e.m.shortcutFor(cs); do != nil {
			do()
			return
		}
	}
	e.Entry.TypedShortcut(s)
}
