package ui

import (
	"fyne.io/fyne/v2/dialog"

	"github.com/idct/helena/internal/model"
)

// editCollectionVariables opens the variables editor for the ACTIVE collection's
// collection-level variables (#80) — a resolver scope below the environment, so
// an environment value of the same name wins. Saved straight back to the
// collection's YAML (secrets externalized like environment variables).
func (m *MainUI) editCollectionVariables() {
	if m.win == nil {
		return
	}
	ci := m.sess.ActiveCollection()
	if ci < 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	cols := m.sess.Collections()
	col := &cols[ci] // live pointer into the session's collection slice
	m.showVariablesEditor("Collection variables: "+col.Name, "Save collection variables", col.Variables,
		func(vars []model.Variable) {
			col.Variables = vars
			if err := m.sess.SaveActiveCollection(); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.updateURLPreview()
			m.Status.SetText("Saved collection variables")
		})
}
