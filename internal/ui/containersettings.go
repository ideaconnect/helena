package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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

// containerSettings is the Variables + Auth settings dialog for a container
// (a folder or a collection root), built by newContainerSettings and shown by
// showContainerSettings. Both editors work on copies; save hands the edited
// values to onSave in one call so the caller persists with a single write.
type containerSettings struct {
	title, guardLabel string
	vars              *variablesEditor
	auth              *authEditor
	authWork          model.Auth // the auth editor's target (a copy of the initial value)
	content           fyne.CanvasObject
	onSave            func(vars []model.Variable, a model.Auth)
}

// newContainerSettings builds the two tabs. allowInherit / inheritText /
// tokenNamespace configure the auth editor (see authEditorOpts).
func (m *MainUI) newContainerSettings(title, guardLabel string, vars []model.Variable, a model.Auth,
	allowInherit bool, inheritText, tokenNamespace func() string,
	onSave func([]model.Variable, model.Auth)) *containerSettings {
	cs := &containerSettings{title: title, guardLabel: guardLabel, authWork: a.Clone(), onSave: onSave}
	cs.vars = m.newVariablesEditor(vars)
	cs.auth = newAuthEditor(m, authEditorOpts{
		target:         func() *model.Auth { return &cs.authWork },
		inheritText:    inheritText,
		allowInherit:   allowInherit,
		tokenNamespace: tokenNamespace,
	})
	cs.auth.load(cs.authWork)
	cs.content = container.NewAppTabs(
		container.NewTabItem("Variables", cs.vars.content),
		container.NewTabItem("Auth", cs.auth.root),
	)
	return cs
}

// save hands the edited variables and auth to onSave.
func (cs *containerSettings) save() {
	cs.onSave(cs.vars.result(), cs.authWork)
}

// showContainerSettings opens cs as a Save / Cancel dialog.
func (m *MainUI) showContainerSettings(cs *containerSettings) {
	d := dialog.NewCustomConfirm(cs.title, "Save", "Cancel", cs.content, func(ok bool) {
		if !ok {
			return
		}
		m.guard(cs.guardLabel, cs.save)
	}, m.win)
	d.Resize(fyne.NewSize(600, 500))
	d.Show()
}

// folderSettings builds the settings dialog for the folder at nodeID: its
// folder-scoped variables (#81, a resolver scope between environment and
// request) and its auth (inherited by every descendant whose own auth is
// Inherit). Returns false when nodeID is not a folder. Saving persists both in
// one write via Session.UpdateFolder (secrets externalized as usual).
func (m *MainUI) folderSettings(nodeID string) (*containerSettings, bool) {
	fvars, ok := m.sess.FolderVariables(nodeID)
	if !ok {
		return nil, false
	}
	fauth, _ := m.sess.FolderAuth(nodeID)
	name := m.sess.Tree().Label(nodeID)
	ci := m.sess.Tree().CollectionIndex(nodeID)
	cs := m.newContainerSettings("Folder settings: "+name, "Save folder settings", fvars, fauth, true,
		func() string { return inheritPreview(m.sess.InheritedAuth(nodeID)) },
		func() string { return m.sess.CollectionDir(ci) },
		func(vars []model.Variable, a model.Auth) {
			err := m.sess.UpdateFolder(nodeID, func(f *model.Folder) {
				f.Variables = vars
				f.Auth = a
			})
			if err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.updateURLPreview()
			m.refreshAuthInheritLabel()
			m.Status.SetText("Saved folder settings")
		})
	return cs, true
}

// editFolderSettings opens the settings dialog for the selected folder.
// Reached from the sidebar's folder-tree button, which refreshSidebarActions
// gates to a folder selection.
func (m *MainUI) editFolderSettings() {
	if m.win == nil {
		return
	}
	cs, ok := m.folderSettings(m.lastSelectedNodeID)
	if !ok {
		m.Status.SetText("Select a folder first")
		return
	}
	m.showContainerSettings(cs)
}

// collectionSettings builds the settings dialog for the collection at index
// ci: its collection-level variables (#80, a resolver scope below the
// environment) and its root auth — the outermost ancestor in the inheritance
// walk, so "Inherit from parent" is not offered (a root has no parent).
// Returns false for an out-of-range index. Saving persists both in one write
// via Session.UpdateCollection.
func (m *MainUI) collectionSettings(ci int) (*containerSettings, bool) {
	cols := m.sess.Collections()
	if ci < 0 || ci >= len(cols) {
		return nil, false
	}
	col := cols[ci]
	cs := m.newContainerSettings("Collection settings: "+col.Name, "Save collection settings", col.Variables, col.Auth, false,
		nil,
		func() string { return m.sess.CollectionDir(ci) },
		func(vars []model.Variable, a model.Auth) {
			err := m.sess.UpdateCollection(ci, func(c *model.Collection) {
				c.Variables = vars
				c.Auth = a
			})
			if err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.updateURLPreview()
			m.refreshAuthInheritLabel()
			m.Status.SetText("Saved collection settings")
		})
	return cs, true
}

// editCollectionSettings opens the settings dialog for the ACTIVE collection.
// Reached from the sidebar toolbar's sliders button.
func (m *MainUI) editCollectionSettings() {
	if m.win == nil {
		return
	}
	cs, ok := m.collectionSettings(m.sess.ActiveCollection())
	if !ok {
		m.Status.SetText("Open a collection first")
		return
	}
	m.showContainerSettings(cs)
}
