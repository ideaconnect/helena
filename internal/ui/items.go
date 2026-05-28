package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// parentForNew returns the container ID to add a new item into based on the
// currently selected tree node. Selecting a folder/collection adds inside it;
// selecting a request adds as a sibling; with nothing selected, falls back to
// the active collection's root.
func (m *MainUI) parentForNew() string {
	if m.lastSelectedNodeID != "" {
		id := m.lastSelectedNodeID
		i := strings.LastIndex(id, "/")
		last := id
		if i >= 0 {
			last = id[i+1:]
		}
		if strings.HasPrefix(last, "r") && i >= 0 {
			return id[:i]
		}
		return id
	}
	if ci := m.sess.ActiveCollection(); ci >= 0 {
		return strconv.Itoa(ci)
	}
	return ""
}

func (m *MainUI) actionNewRequest() {
	if m.win == nil {
		return
	}
	parent := m.parentForNew()
	if parent == "" {
		m.Status.SetText("Open a collection first")
		return
	}
	m.promptName("New request", "Request name", "Untitled Request", func(name string) {
		newID, err := m.sess.AddRequest(parent, name)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.Tree.Select(newID)
		m.Status.SetText("Added request: " + name)
	})
}

func (m *MainUI) actionNewFolder() {
	if m.win == nil {
		return
	}
	parent := m.parentForNew()
	if parent == "" {
		m.Status.SetText("Open a collection first")
		return
	}
	m.promptName("New folder", "Folder name", "Untitled", func(name string) {
		newID, err := m.sess.AddFolder(parent, name)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.Tree.OpenBranch(newID)
		m.Status.SetText("Added folder: " + name)
	})
}

func (m *MainUI) actionRename() {
	if m.win == nil {
		return
	}
	id := m.lastSelectedNodeID
	if id == "" {
		m.Status.SetText("Select an item to rename")
		return
	}
	current := nameOfNode(m, id)
	m.promptName("Rename", "New name", current, func(name string) {
		if err := m.sess.RenameItem(id, name); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.Status.SetText("Renamed: " + name)
	})
}

func (m *MainUI) actionDelete() {
	if m.win == nil {
		return
	}
	id := m.lastSelectedNodeID
	if id == "" {
		m.Status.SetText("Select an item to delete")
		return
	}
	label := nameOfNode(m, id)
	dialog.ShowConfirm("Delete?",
		fmt.Sprintf("Delete %q? This cannot be undone.", label),
		func(yes bool) {
			if !yes {
				return
			}
			// If the doomed node is the currently open request (or contains it),
			// clear the editor so we don't keep a stale pointer.
			if id == m.currentRequestID || isAncestor(id, m.currentRequestID) {
				m.loadRequest(nil, "")
			}
			if err := m.sess.DeleteItem(id); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.lastSelectedNodeID = ""
			m.Tree.Refresh()
			m.Status.SetText("Deleted: " + label)
		}, m.win)
}

func (m *MainUI) actionDuplicate() {
	if m.win == nil {
		return
	}
	id := m.lastSelectedNodeID
	if id == "" {
		m.Status.SetText("Select an item to duplicate")
		return
	}
	newID, err := m.sess.DuplicateItem(id)
	if err != nil {
		dialog.ShowError(err, m.win)
		return
	}
	m.Tree.Refresh()
	m.Tree.Select(newID)
	m.Status.SetText("Duplicated")
}

// promptName shows a single-field dialog for entering a name, calling onAccept
// with the trimmed value when the user confirms (and the value isn't empty).
func (m *MainUI) promptName(title, label, initial string, onAccept func(string)) {
	entry := widget.NewEntry()
	entry.SetText(initial)
	form := widget.NewForm(widget.NewFormItem(label, entry))
	d := dialog.NewCustomConfirm(title, "OK", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		name := strings.TrimSpace(entry.Text)
		if name == "" {
			m.Status.SetText("Name cannot be empty")
			return
		}
		onAccept(name)
	}, m.win)
	d.Resize(fyne.NewSize(380, 160))
	d.Show()
}

func nameOfNode(m *MainUI, id string) string {
	// For requests, show the raw name (no "GET  " prefix); fall back to Label
	// for collections and folders.
	i := strings.LastIndex(id, "/")
	last := id
	if i >= 0 {
		last = id[i+1:]
	}
	if strings.HasPrefix(last, "r") {
		if r, ok := m.sess.Tree().Request(id); ok {
			return r.Name
		}
	}
	return m.sess.Tree().Label(id)
}

// isAncestor reports whether ancestor's node ID is a prefix path of descendant.
func isAncestor(ancestor, descendant string) bool {
	if ancestor == "" || descendant == "" {
		return false
	}
	return ancestor == descendant || strings.HasPrefix(descendant, ancestor+"/")
}
