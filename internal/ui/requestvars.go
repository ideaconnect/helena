package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// enabledRequestVars flattens a request's variables into a name->value map,
// skipping disabled rows. Used as the highest-precedence static resolver scope
// for the request being sent (#82).
func enabledRequestVars(vs []model.Variable) map[string]string {
	m := make(map[string]string, len(vs))
	for _, v := range vs {
		if v.Enabled {
			m[v.Key] = v.Value
		}
	}
	return m
}

// buildVarsTab is the request editor's "Vars" tab — request-scoped variables
// (#82), the highest-precedence static scope. The editor reads the live
// currentRequest when opened, so it always targets the loaded request without
// per-load wiring; edits land on currentRequest and persist on the next Save.
func (m *MainUI) buildVarsTab() fyne.CanvasObject {
	help := widget.NewLabel("Request variables resolve {{name}} templates at the highest static\n" +
		"precedence — above environment and collection variables (only a\n" +
		"script's helena.env.set overlay wins). They apply only to this request.")
	edit := widget.NewButtonWithIcon("Edit request variables…", themedIcon("table-list"), m.editRequestVariables)
	return container.NewBorder(container.NewVBox(help, edit), nil, nil, nil, nil)
}

// editRequestVariables opens the shared variables editor bound to the loaded
// request's variables. Writes land on currentRequest; the normal request Save
// persists them (with secret values externalized like collection / env vars).
func (m *MainUI) editRequestVariables() {
	if m.win == nil {
		return
	}
	if m.currentRequest == nil {
		m.Status.SetText("Open a request first")
		return
	}
	r := m.currentRequest
	m.showVariablesEditor("Request variables: "+r.Name, "Save request variables", r.Variables,
		func(vars []model.Variable) {
			r.Variables = vars
			m.Status.SetText("Updated request variables — press Save to persist")
		})
}
