package ui

import (
	"strings"

	"fyne.io/fyne/v2/dialog"

	"github.com/idct/helena/internal/model"
)

// isFolderSelected reports whether the selected tree node is a folder (a branch
// with a parent — i.e. not a collection root and not a request).
func (m *MainUI) isFolderSelected() bool {
	sel := m.lastSelectedNodeID
	if !strings.Contains(sel, "/") {
		return false // empty or a collection root
	}
	_, isRequest := m.sess.Tree().Request(sel)
	return !isRequest
}

// editFolderVariables opens the variables editor for the selected folder's
// folder-scoped variables (#81) — a resolver scope between the environment and
// the request, with inner folders overriding outer ones. Saved back to the
// folder's collection (secrets externalized like environment variables).
func (m *MainUI) editFolderVariables() {
	if m.win == nil {
		return
	}
	sel := m.lastSelectedNodeID
	fvars, ok := m.sess.FolderVariables(sel)
	if !ok {
		m.Status.SetText("Select a folder first")
		return
	}
	name := m.sess.Tree().Label(sel)
	m.showVariablesEditor("Folder variables: "+name, "Save folder variables", fvars,
		func(vars []model.Variable) {
			if err := m.sess.SetFolderVariables(sel, vars); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.updateURLPreview()
			m.Status.SetText("Saved folder variables")
		})
}
