package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
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

// editEnvironments opens a simple "key = value" editor for the active
// collection's active environment, creating one if needed, and saves changes
// back to the collection's YAML on disk.
// envSecretMask is the placeholder shown in the environment editor in place of
// a Secret variable's value until the user reveals it (#43). Left in place on
// save, it means "keep the stored value" so editing other lines never clobbers
// a hidden secret.
const envSecretMask = "•••••••• (secret — reveal to edit)"

// maskedEnvText renders env vars as `key = value` lines, replacing each Secret
// variable's value with envSecretMask unless reveal is true.
func maskedEnvText(vars []model.Variable, reveal bool) string {
	if reveal {
		return session.FormatEnvVars(vars)
	}
	masked := make([]model.Variable, len(vars))
	copy(masked, vars)
	for i := range masked {
		if masked[i].Secret {
			masked[i].Value = envSecretMask
		}
	}
	return session.FormatEnvVars(masked)
}

// restoreEnvSecrets re-marks parsed env vars whose key was a known secret as
// Secret, and restores the stored value wherever the user left envSecretMask in
// place (so editing other lines without revealing never clobbers a secret). A
// value that differs from the mask is taken as the user's new value.
func restoreEnvSecrets(parsed []model.Variable, secretVals map[string]string) []model.Variable {
	for i := range parsed {
		if storedVal, ok := secretVals[parsed[i].Key]; ok {
			parsed[i].Secret = true
			if parsed[i].Value == envSecretMask {
				parsed[i].Value = storedVal
			}
		}
	}
	return parsed
}

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

	// Stored secret values, keyed by var key — used to restore an unrevealed
	// secret on save and to reveal real values on toggle.
	secretVals := map[string]string{}
	for _, v := range env.Variables {
		if v.Secret {
			secretVals[v.Key] = v.Value
		}
	}

	entry := widget.NewMultiLineEntry()
	entry.SetText(maskedEnvText(env.Variables, false))
	entry.SetMinRowsVisible(10)

	label := widget.NewLabel("One per line:  key = value   (prefix with # to disable)")
	var top fyne.CanvasObject = label
	if len(secretVals) > 0 {
		// Reveal re-renders from the saved variables (masked vs. cleartext);
		// in-textarea edits made before toggling are reloaded from saved state.
		reveal := widget.NewCheck("Reveal secret values", func(on bool) {
			entry.SetText(maskedEnvText(env.Variables, on))
		})
		top = container.NewVBox(label, reveal)
	}
	content := container.NewBorder(top, nil, nil, nil, entry)

	d := dialog.NewCustomConfirm("Environment: "+env.Name, "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		m.guard("Save environment", func() {
			parsed := restoreEnvSecrets(session.ParseEnvVars(entry.Text), secretVals)
			m.sess.SetActiveEnvironmentVariables(parsed)
			if err := m.sess.SaveActiveCollection(); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.refreshEnvironments()
			m.updateURLPreview()
			m.Status.SetText("Saved environment: " + env.Name)
		})
	}, m.win)
	d.Resize(fyne.NewSize(540, 400))
	d.Show()
}
