package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// parentForNew returns the container ID to add a new item into based
// on the currently selected tree node. Selecting a folder/collection
// adds inside it; selecting a request adds as a sibling; with nothing
// selected, falls back to the active collection's root. Returns ""
// when no collection is loaded — callers (the keyboard shortcut path)
// should no-op in that case.
//
// The per-row + Req / + Folder icons take a parent ID directly (the
// row they live on) so they don't go through this helper.
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

// actionNewRequest is the keyboard-shortcut entry point — adds a
// request to the row inferred by parentForNew. Quietly no-ops when
// there's no collection to add into (no UI thrash from a stray N
// keystroke before a workspace is opened).
func (m *MainUI) actionNewRequest() {
	if parent := m.parentForNew(); parent != "" {
		m.addRequestUnder(parent)
	}
}

// addRequestUnder prompts for a name and adds a request as a child
// of the given parent (collection or folder). Shared by the per-row
// add-request icon — there is no global add-request affordance any
// more; every container row carries its own.
func (m *MainUI) addRequestUnder(parent string) {
	if m.win == nil || parent == "" {
		return
	}
	m.promptName("New request", "Request name", "Untitled Request", func(name string) {
		newID, err := m.sess.AddRequest(parent, name)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.reconcileTabs() // the append may have shifted sibling node IDs
		m.Tree.OpenBranch(parent)
		m.Tree.Select(newID)
		m.Status.SetText("Added request: " + name)
	})
}

// addFolderUnder is the folder analogue of addRequestUnder.
func (m *MainUI) addFolderUnder(parent string) {
	if m.win == nil || parent == "" {
		return
	}
	m.promptName("New folder", "Folder name", "Untitled", func(name string) {
		newID, err := m.sess.AddFolder(parent, name)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.Tree.OpenBranch(parent)
		m.Tree.OpenBranch(newID)
		m.Status.SetText("Added folder: " + name)
	})
}

// actionRename / actionDelete / actionDuplicate are the
// keyboard-shortcut + global-button entry points; they operate on the
// last-selected node. They delegate to the renameNode / deleteNode /
// duplicateNode helpers that the per-row tree icons also call so all
// paths share the same prompt + confirmation flow.
func (m *MainUI) actionRename() {
	if m.lastSelectedNodeID == "" {
		m.Status.SetText("Select an item to rename")
		return
	}
	m.renameNode(m.lastSelectedNodeID)
}

func (m *MainUI) actionDelete() {
	if m.lastSelectedNodeID == "" {
		m.Status.SetText("Select an item to delete")
		return
	}
	m.deleteNode(m.lastSelectedNodeID)
}

func (m *MainUI) actionDuplicate() {
	if m.lastSelectedNodeID == "" {
		m.Status.SetText("Select an item to duplicate")
		return
	}
	m.duplicateNode(m.lastSelectedNodeID)
}

// renameNode prompts for a new name and applies it to the node at id.
// Shared by the global Rename button + per-row rename icons.
func (m *MainUI) renameNode(id string) {
	if m.win == nil || id == "" {
		return
	}
	current := nameOfNode(m, id)
	m.promptName("Rename", "New name", current, func(name string) {
		if err := m.sess.RenameItem(id, name); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.reconcileTabs() // refresh any open tab's label for the renamed request
		m.Status.SetText("Renamed: " + name)
	})
}

// deleteNode asks for confirmation, then deletes the node at id.
// Centralised so the global Delete button and the per-row delete
// icons share the same confirmation dialog (invariant from 9.13:
// every deletion must confirm).
//
// Collection-level IDs (no "/") route to Session.RemoveCollection
// so the on-disk dir is left intact — Postman/Bruno convention is
// "remove from this workspace", not "delete the files". Folder /
// request IDs go through Session.DeleteItem which does mutate the
// tree on disk.
func (m *MainUI) deleteNode(id string) {
	if m.win == nil || id == "" {
		return
	}
	label := nameOfNode(m, id)
	isCollection := !strings.Contains(id, "/")
	prompt := fmt.Sprintf("Delete %q? This cannot be undone.", label)
	if isCollection {
		prompt = fmt.Sprintf("Remove collection %q from this workspace?\nThe folder on disk stays untouched.", label)
	}
	dialog.ShowConfirm("Delete?", prompt, func(yes bool) {
		if !yes {
			return
		}
		// If the doomed node is the currently open request (or
		// contains it), clear the editor so we don't keep a stale
		// pointer into the deleted subtree.
		if id == m.currentRequestID || isAncestor(id, m.currentRequestID) {
			m.loadRequest(nil, "")
		}
		var err error
		if isCollection {
			ci, convErr := strconv.Atoi(id)
			if convErr != nil {
				dialog.ShowError(fmt.Errorf("invalid collection id %q: %w", id, convErr), m.win)
				return
			}
			err = m.sess.RemoveCollection(ci)
		} else {
			err = m.sess.DeleteItem(id)
		}
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if m.lastSelectedNodeID == id {
			m.lastSelectedNodeID = ""
		}
		m.Tree.Refresh()
		// Drop tabs for the deleted request (or every request in a removed
		// collection) and remap surviving tabs' shifted node IDs.
		m.reconcileTabs()
		if isCollection {
			m.refreshEnvironments()
			m.Status.SetText("Removed collection: " + label)
		} else {
			m.Status.SetText("Deleted: " + label)
		}
	}, m.win)
}

// duplicateNode clones the node at id and selects the new copy. Shared
// by the global Duplicate button (now removed in favour of row icons)
// and the per-row copy icon on request rows.
func (m *MainUI) duplicateNode(id string) {
	if m.win == nil || id == "" {
		return
	}
	newID, err := m.sess.DuplicateItem(id)
	if err != nil {
		dialog.ShowError(err, m.win)
		return
	}
	m.Tree.Refresh()
	m.reconcileTabs() // the insert shifted later siblings' node IDs
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

// nameOfNode returns the display name suitable for a Rename/Delete prompt:
// the raw request name for requests, or the tree label for folders and
// collections.
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
