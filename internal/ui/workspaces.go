package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// editWorkspaces opens a list-style dialog for adding/renaming/deleting
// workspaces. Changes are persisted immediately via the session.
func (m *MainUI) editWorkspaces() {
	if m.win == nil {
		return
	}
	selectedIdx := -1
	list := widget.NewList(
		func() int { return len(m.sess.WorkspaceNames()) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(m.sess.WorkspaceNames()[i])
		},
	)
	// Rename / Delete act on the selected row, so they stay disabled until one
	// is selected (#130); selectionChanged keeps them in sync with the list.
	var renameBtn, deleteBtn *ttwidget.Button
	selectionChanged := func() {
		enableButton(renameBtn, selectedIdx >= 0)
		enableButton(deleteBtn, selectedIdx >= 0)
	}
	list.OnSelected = func(i widget.ListItemID) { selectedIdx = i; selectionChanged() }
	list.OnUnselected = func(widget.ListItemID) { selectedIdx = -1; selectionChanged() }

	addBtn := tipButton("square-plus", "Add workspace", func() {
		m.promptName("New workspace", "Name", "", func(name string) {
			if err := m.sess.AddWorkspace(name); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			list.Refresh()
			m.refreshWorkspaceDropdown()
		})
	})
	renameBtn = tipButton("pen-to-square", "Rename workspace", func() {
		if selectedIdx < 0 {
			return
		}
		current := m.sess.WorkspaceNames()[selectedIdx]
		idx := selectedIdx
		m.promptName("Rename workspace", "Name", current, func(name string) {
			if err := m.sess.RenameWorkspace(idx, name); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			list.Refresh()
			m.refreshWorkspaceDropdown()
		})
	})
	deleteBtn = tipButton("trash-can", "Delete workspace", func() {
		if selectedIdx < 0 {
			return
		}
		name := m.sess.WorkspaceNames()[selectedIdx]
		idx := selectedIdx
		dialog.ShowConfirm("Delete workspace?",
			fmt.Sprintf("Delete workspace %q? Its collection list is removed; collection folders on disk stay.", name),
			func(yes bool) {
				if !yes {
					return
				}
				if err := m.sess.DeleteWorkspace(idx); err != nil {
					dialog.ShowError(err, m.win)
					return
				}
				selectedIdx = -1
				list.UnselectAll()
				list.Refresh()
				selectionChanged()
				m.refreshWorkspaceDropdown()
				m.refreshEnvironments()
				m.Tree.Refresh()
				m.loadRequest(nil, "")
			}, m.win)
	})
	selectionChanged() // nothing selected yet → Rename/Delete start disabled

	// Action buttons on top, right-aligned (the spacer pushes them right).
	actions := container.NewHBox(layout.NewSpacer(), addBtn, renameBtn, deleteBtn)
	content := container.NewBorder(actions, nil, nil, nil, list)
	d := dialog.NewCustom("Workspaces", "Done", content, m.win)
	d.Resize(fyne.NewSize(440, 320))
	d.Show()
}

// refreshWorkspaceDropdown reseeds the toolbar's workspace Select from session.
func (m *MainUI) refreshWorkspaceDropdown() {
	names := m.sess.WorkspaceNames()
	m.Workspace.Options = names
	if len(names) > 0 && m.sess.ActiveIndex() < len(names) {
		m.Workspace.SetSelected(names[m.sess.ActiveIndex()])
	}
	m.Workspace.Refresh()
	m.updateWindowTitle()
}
