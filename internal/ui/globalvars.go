package ui

import (
	"fyne.io/fyne/v2/dialog"

	"github.com/idct/helena/internal/model"
)

// editGlobalVariables opens the variables editor for the app-wide global
// variables (#83) — the lowest-precedence resolver scope, shared across every
// collection and persisted in the config. Saved straight back to the config;
// secret values are stored inline there (the config is local app state, not a
// shared collection artifact — externalization is a possible follow-up).
func (m *MainUI) editGlobalVariables() {
	if m.win == nil {
		return
	}
	m.showVariablesEditor("Global variables", "Save global variables", m.sess.GlobalVariables(),
		func(vars []model.Variable) {
			if err := m.sess.SetGlobalVariables(vars); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.updateURLPreview()
			m.Status.SetText("Saved global variables")
		})
}
