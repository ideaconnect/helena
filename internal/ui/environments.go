package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// manageEnvironments opens a manager for the active collection's named
// environments: create, rename, delete, and set-active. Per-environment
// variable editing stays in editEnvironments (the Variables icon button). Every mutation
// round-trips through the session, which persists to the collection's YAML.
func (m *MainUI) manageEnvironments() {
	if m.win == nil {
		return
	}
	if m.sess.ActiveCollection() < 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	selectedIdx := -1
	names := func() []string { return m.sess.CollectionEnvironmentNames() }

	list := widget.NewList(
		func() int { return len(names()) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			n := names()
			if i < 0 || i >= len(n) {
				return
			}
			label := n[i]
			if label == m.sess.ActiveEnvName() {
				label += "  (active)"
			}
			o.(*widget.Label).SetText(label)
		},
	)

	// Rename / Delete / Set-active act on the selected row, so they stay disabled
	// until one is selected (matching the Workspaces dialog).
	var renameBtn, deleteBtn, activateBtn *ttwidget.Button
	selectionChanged := func() {
		on := selectedIdx >= 0
		enableButton(renameBtn, on)
		enableButton(deleteBtn, on)
		enableButton(activateBtn, on)
	}
	list.OnSelected = func(i widget.ListItemID) { selectedIdx = i; selectionChanged() }
	list.OnUnselected = func(widget.ListItemID) { selectedIdx = -1; selectionChanged() }

	after := func() {
		list.Refresh()
		m.refreshEnvironments()
		m.updateURLPreview()
	}
	selected := func() (string, bool) {
		n := names()
		if selectedIdx < 0 || selectedIdx >= len(n) {
			return "", false
		}
		return n[selectedIdx], true
	}

	addBtn := tipButton("square-plus", "New environment", func() {
		m.promptName("New environment", "Name", "", func(name string) {
			if err := m.sess.AddEnvironment(name); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			after()
		})
	})
	renameBtn = tipButton("pen-to-square", "Rename environment", func() {
		old, ok := selected()
		if !ok {
			return
		}
		m.promptName("Rename environment", "Name", old, func(name string) {
			if err := m.sess.RenameEnvironment(old, name); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			after()
		})
	})
	deleteBtn = tipButton("trash-can", "Delete environment", func() {
		name, ok := selected()
		if !ok {
			return
		}
		dialog.ShowConfirm("Delete environment?",
			fmt.Sprintf("Delete environment %q and its variables?", name),
			func(yes bool) {
				if !yes {
					return
				}
				if err := m.sess.DeleteEnvironment(name); err != nil {
					dialog.ShowError(err, m.win)
					return
				}
				selectedIdx = -1
				list.UnselectAll()
				selectionChanged()
				after()
			}, m.win)
	})
	activateBtn = tipButtonRes(theme.ConfirmIcon(), "Set active", func() {
		name, ok := selected()
		if !ok {
			return
		}
		m.sess.SetActiveEnv(name)
		after()
	})
	selectionChanged() // nothing selected yet → Rename/Delete/Set-active disabled

	// Action buttons on top, right-aligned (the spacer pushes them right).
	buttons := container.NewHBox(layout.NewSpacer(), addBtn, renameBtn, deleteBtn, activateBtn)
	content := container.NewBorder(buttons, nil, nil, nil, list)
	d := dialog.NewCustom("Environments", "Close", content, m.win)
	d.Resize(fyne.NewSize(440, 380))
	d.Show()
}
