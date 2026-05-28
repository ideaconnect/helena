package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
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
	list.OnSelected = func(i widget.ListItemID) { selectedIdx = i }
	list.OnUnselected = func(widget.ListItemID) { selectedIdx = -1 }

	addBtn := widget.NewButton("+ Add", func() {
		m.promptName("New workspace", "Name", "", func(name string) {
			if err := m.sess.AddWorkspace(name); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			list.Refresh()
			m.refreshWorkspaceDropdown()
		})
	})
	renameBtn := widget.NewButton("Rename", func() {
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
	deleteBtn := widget.NewButton("Delete", func() {
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
				m.refreshWorkspaceDropdown()
				m.refreshEnvironments()
				m.Tree.Refresh()
				m.loadRequest(nil, "")
			}, m.win)
	})

	actions := container.NewHBox(addBtn, renameBtn, deleteBtn)
	content := container.NewBorder(nil, actions, nil, nil, list)
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
}
