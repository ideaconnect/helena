package ui

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// onEnvChanged is the Environment dropdown's selection handler; it stores the
// chosen environment on the session and re-runs the URL preview so its
// resolution reflects the new variables.
func (m *MainUI) onEnvChanged(name string) {
	if name == noEnv {
		m.sess.SetActiveEnv("")
	} else {
		m.sess.SetActiveEnv(name)
	}
	m.updateURLPreview()
}

// refreshEnvironments reseeds the Environment dropdown from the active
// collection's environment list and restores the previously selected name.
// Tolerates being called before the Environment widget is built (e.g.
// during Workspace dropdown's initial selection inside NewMainUI).
func (m *MainUI) refreshEnvironments() {
	if m.Environment == nil {
		return
	}
	m.Environment.Options = append([]string{noEnv}, m.sess.CollectionEnvironmentNames()...)
	sel := m.sess.ActiveEnvName()
	if sel == "" {
		sel = noEnv
	}
	m.Environment.SetSelected(sel)
	m.Environment.Refresh()
}

// envSecretMask is the placeholder shown in a Secret variable's value field
// until the user reveals it (#43). The editor keeps the real values in a working
// copy and masks only the *display*; an un-revealed secret field is also
// disabled, so editing other rows can never clobber the hidden value.
const envSecretMask = "•••••••• (secret — reveal to edit)"

// varRowValue is the text shown in a variable row's value entry: the real value,
// or the placeholder for a Secret variable that has not been revealed.
func varRowValue(v model.Variable, reveal bool) string {
	if v.Secret && !reveal {
		return envSecretMask
	}
	return v.Value
}

// pruneEmptyVars drops rows whose key trims to empty — blank "Add variable" rows
// the user never filled in — so they don't reach storage.
func pruneEmptyVars(vars []model.Variable) []model.Variable {
	out := vars[:0]
	for _, v := range vars {
		if strings.TrimSpace(v.Key) != "" {
			out = append(out, v)
		}
	}
	return out
}

// buildVarRow renders one editable variable row (enable check, key, value,
// delete) writing back into vars by index — mirroring the headers key/value
// editor (buildKVRow). A Secret value shows the mask and stays disabled until
// reveal, so it can't be edited or cleared blind (#43). refresh rebuilds the
// row list after a delete.
func (m *MainUI) buildVarRow(vars *[]model.Variable, idx int, reveal bool, refresh func()) fyne.CanvasObject {
	v := (*vars)[idx]
	// OnChanged handlers are assigned AFTER SetText/SetChecked so seeding the
	// widgets during a rebuild doesn't fire write-backs.
	check := widget.NewCheck("", nil)
	check.SetChecked(v.Enabled)
	check.OnChanged = func(b bool) {
		if idx < len(*vars) {
			(*vars)[idx].Enabled = b
		}
	}
	keyEntry := widget.NewEntry()
	keyEntry.SetPlaceHolder("key")
	keyEntry.SetText(v.Key)
	keyEntry.OnChanged = func(s string) {
		if idx < len(*vars) {
			(*vars)[idx].Key = s
		}
	}
	valEntry := widget.NewEntry()
	valEntry.SetPlaceHolder("value")
	valEntry.SetText(varRowValue(v, reveal))
	if v.Secret && !reveal {
		valEntry.Disable()
	}
	valEntry.OnChanged = func(s string) {
		if idx < len(*vars) {
			(*vars)[idx].Value = s
		}
	}
	// Same affordance as the headers editor: a low-importance circle-xmark.
	delBtn := widget.NewButtonWithIcon("", themedIcon("circle-xmark"), func() {
		if idx < len(*vars) {
			*vars = slices.Delete(*vars, idx, idx+1)
		}
		refresh()
	})
	delBtn.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, check, delBtn,
		container.NewGridWithColumns(2, keyEntry, valEntry))
}

// editEnvironments opens a key/value list editor (like the Headers tab) for the
// active collection's active environment, creating one if needed, and saves
// changes back to the collection's YAML. Unchecked rows are kept but marked
// disabled (replacing the old `# key = value` syntax); Secret values are masked
// until revealed (#43).
func (m *MainUI) editEnvironments() {
	if m.win == nil {
		return
	}
	if m.sess.ActiveCollection() < 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	if m.sess.ActiveEnvName() == "" {
		if names := m.sess.CollectionEnvironmentNames(); len(names) > 0 {
			m.sess.SetActiveEnv(names[0])
		} else {
			_ = m.sess.AddEnvironment("Default")
			m.sess.SetActiveEnv("Default")
		}
	}
	env := m.sess.ActiveEnvironment()
	if env == nil {
		m.Status.SetText("No environment to edit")
		return
	}

	// Working copy holds the real values; the list masks secrets in the display.
	vars := append([]model.Variable(nil), env.Variables...)
	hasSecret := false
	for _, v := range vars {
		if v.Secret {
			hasSecret = true
			break
		}
	}

	reveal := false
	rows := container.NewVBox()
	var rebuild func()
	rebuild = func() {
		rows.RemoveAll()
		for i := range vars {
			rows.Add(m.buildVarRow(&vars, i, reveal, rebuild))
		}
		rows.Refresh()
	}
	rebuild()

	addBtn := tipButton("square-plus", "Add variable", func() {
		vars = append(vars, model.Variable{Enabled: true})
		rebuild()
	})

	// Add button right-aligned (matching the workspaces dialog); the reveal
	// toggle, when present, sits at the left of the same row.
	var top fyne.CanvasObject = container.NewBorder(nil, nil, nil, addBtn, nil)
	if hasSecret {
		// Reveal re-renders secret rows with their real (editable) values.
		revealCheck := widget.NewCheck("Reveal secret values", func(on bool) {
			reveal = on
			rebuild()
		})
		top = container.NewBorder(nil, nil, revealCheck, addBtn, nil)
	}
	content := container.NewBorder(top, nil, nil, nil, container.NewVScroll(rows))

	d := dialog.NewCustomConfirm("Environment: "+env.Name, "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		m.guard("Save environment", func() {
			m.sess.SetActiveEnvironmentVariables(pruneEmptyVars(vars))
			if err := m.sess.SaveActiveCollection(); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.refreshEnvironments()
			m.updateURLPreview()
			m.Status.SetText("Saved environment: " + env.Name)
		})
	}, m.win)
	d.Resize(fyne.NewSize(560, 440))
	d.Show()
}
