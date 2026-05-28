package ui

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// shortcutSpec describes one keyboard binding. label is the human-readable key
// (e.g. "Enter", "S", "Shift+N") shown in the help dialog alongside the
// platform's modifier name (Ctrl on Linux/Windows, ⌘ on macOS). extraMod is
// OR'd with fyne.KeyModifierShortcutDefault when registering — leave zero for
// plain Mod+key bindings, set to fyne.KeyModifierShift for Mod+Shift+key, etc.
type shortcutSpec struct {
	keyName  fyne.KeyName
	extraMod fyne.KeyModifier
	label    string
	action   string
	do       func()
}

// registerShortcuts wires application-wide keyboard bindings against the
// window's canvas. Every binding uses fyne.KeyModifierShortcutDefault so it
// resolves to Ctrl on Linux/Windows and ⌘ on macOS automatically.
func (m *MainUI) registerShortcuts() {
	if m.win == nil {
		return
	}
	m.shortcuts = []shortcutSpec{
		{fyne.KeyReturn, 0, "Enter", "Send request", m.send},
		{fyne.KeyS, 0, "S", "Save request", m.saveRequest},
		{fyne.KeyO, 0, "O", "Open collection", m.openCollection},
		{fyne.KeyI, 0, "I", "Import (OpenAPI / WSDL)", m.actionImport},
		{fyne.KeyN, 0, "N", "New request", m.actionNewRequest},
		{fyne.KeyN, fyne.KeyModifierShift, "Shift+N", "New collection", m.actionNewCollection},
		{fyne.KeyD, 0, "D", "Duplicate selected item", m.actionDuplicate},
		{fyne.KeyE, 0, "E", "Edit environments", m.editEnvironments},
		{fyne.KeyComma, 0, ",", "Settings", m.editSettings},
	}
	c := m.win.Canvas()
	for _, s := range m.shortcuts {
		do := s.do
		c.AddShortcut(&desktop.CustomShortcut{
			KeyName:  s.keyName,
			Modifier: fyne.KeyModifierShortcutDefault | s.extraMod,
		}, func(_ fyne.Shortcut) { do() })
	}
	c.SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyF1 {
			m.showShortcuts()
		}
	})
}

// shortcutModifierName returns the platform-appropriate label for the default
// shortcut modifier (⌘ on macOS, Ctrl elsewhere).
func shortcutModifierName() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl"
}

// showShortcuts opens a non-modal dialog listing every registered shortcut
// alongside its action so users can discover bindings without leaving the app.
func (m *MainUI) showShortcuts() {
	if m.win == nil {
		return
	}
	mod := shortcutModifierName()
	rows := make([]fyne.CanvasObject, 0, len(m.shortcuts)+1)
	for _, s := range m.shortcuts {
		combo := mod + "+" + s.label
		rows = append(rows, shortcutRow(combo, s.action))
	}
	rows = append(rows, shortcutRow("F1", "Show this list"))
	d := dialog.NewCustom("Keyboard shortcuts", "Close",
		container.NewVBox(rows...), m.win)
	d.Resize(fyne.NewSize(420, 360))
	d.Show()
}

func shortcutRow(combo, action string) fyne.CanvasObject {
	key := widget.NewLabelWithStyle(combo, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.New(&shortcutRowLayout{},
		key, widget.NewLabel(action))
}

// shortcutRowLayout lays out a fixed-width key column on the left and lets the
// action label fill the remaining width — simpler than a grid for two columns
// where only one column has a known minimum.
type shortcutRowLayout struct{}

func (l *shortcutRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) != 2 {
		return
	}
	keyMin := objs[0].MinSize()
	keyW := keyMin.Width
	if keyW < 110 {
		keyW = 110
	}
	objs[0].Resize(fyne.NewSize(keyW, size.Height))
	objs[0].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(size.Width-keyW, size.Height))
	objs[1].Move(fyne.NewPos(keyW, 0))
}

func (l *shortcutRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) != 2 {
		return fyne.NewSize(0, 0)
	}
	a, b := objs[0].MinSize(), objs[1].MinSize()
	h := a.Height
	if b.Height > h {
		h = b.Height
	}
	keyW := a.Width
	if keyW < 110 {
		keyW = 110
	}
	return fyne.NewSize(keyW+b.Width, h)
}
