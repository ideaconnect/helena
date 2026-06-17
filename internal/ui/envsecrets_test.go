package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestVarRowValueMasksUnrevealedSecret pins that the variables list masks a
// Secret value until revealed and never masks a non-secret value (#43).
func TestVarRowValueMasksUnrevealedSecret(t *testing.T) {
	sec := model.Variable{Enabled: true, Key: "TOKEN", Value: "s3cret-value", Secret: true}
	pub := model.Variable{Enabled: true, Key: "HOST", Value: "api.test"}

	if got := varRowValue(sec, false); got != envSecretMask {
		t.Errorf("unrevealed secret value = %q; want the mask", got)
	}
	if got := varRowValue(sec, true); got != "s3cret-value" {
		t.Errorf("revealed secret value = %q; want the real value", got)
	}
	if got := varRowValue(pub, false); got != "api.test" {
		t.Errorf("non-secret value = %q; want it shown unmasked", got)
	}
}

// TestPruneEmptyVarsDropsBlankKeys pins that blank-key rows are dropped on save.
func TestPruneEmptyVarsDropsBlankKeys(t *testing.T) {
	in := []model.Variable{
		{Key: "A", Value: "1"},
		{Key: "  ", Value: "x"},
		{Key: "", Value: ""},
		{Key: "B", Value: "2"},
	}
	got := pruneEmptyVars(in)
	if len(got) != 2 || got[0].Key != "A" || got[1].Key != "B" {
		t.Errorf("pruneEmptyVars = %+v; want only A and B", got)
	}
}

// TestEnvEditorMasksSecretUntilReveal opens the real editor dialog and verifies
// the secret value is not present in any entry until the user reveals it (#43),
// while a non-secret value is shown immediately.
func TestEnvEditorMasksSecretUntilReveal(t *testing.T) {
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	if err := storage.Save(model.Collection{Name: "C0"}, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	_ = s.AddEnvironment("Default")
	s.SetActiveEnv("Default")
	s.SetActiveEnvironmentVariables([]model.Variable{
		{Enabled: true, Key: "HOST", Value: "api.test"},
		{Enabled: true, Key: "TOKEN", Value: "s3cret-value", Secret: true},
	})

	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(800, 600))
	defer w.Close()
	m.SetWindow(w)

	m.editEnvironments()
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("environment dialog did not open")
	}

	entryWith := func(text string) bool {
		found := false
		walkObjects(top, func(o fyne.CanvasObject) {
			if e, ok := o.(*widget.Entry); ok && e.Text == text {
				found = true
			}
		})
		return found
	}

	if entryWith("s3cret-value") {
		t.Error("secret value present in an entry before reveal")
	}
	if !entryWith("api.test") {
		t.Error("non-secret value should be shown in an entry")
	}

	var reveal *widget.Check
	walkObjects(top, func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok && c.Text == "Reveal secret values" {
			reveal = c
		}
	})
	if reveal == nil {
		t.Fatal("reveal-secrets checkbox not found")
	}
	reveal.SetChecked(true)

	if !entryWith("s3cret-value") {
		t.Error("secret value should be shown in an entry after reveal")
	}
}

// TestEnvEditorSavePreservesUnrevealedSecret pins the #43 save invariant the
// removed restoreEnvSecrets test used to cover: editing another row and saving
// WITHOUT revealing keeps the secret's real value (never persists the mask).
func TestEnvEditorSavePreservesUnrevealedSecret(t *testing.T) {
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	if err := storage.Save(model.Collection{Name: "C0"}, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	_ = s.AddEnvironment("Default")
	s.SetActiveEnv("Default")
	s.SetActiveEnvironmentVariables([]model.Variable{
		{Enabled: true, Key: "HOST", Value: "api.test"},
		{Enabled: true, Key: "TOKEN", Value: "s3cret-value", Secret: true},
	})

	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(800, 600))
	defer w.Close()
	m.SetWindow(w)

	m.editEnvironments()
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("environment dialog did not open")
	}

	// Edit the NON-secret row and Save without ever revealing the secret.
	var hostEntry *widget.Entry
	var saveBtn *widget.Button
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Entry:
			if v.Text == "api.test" {
				hostEntry = v
			}
		case *widget.Button:
			if v.Text == "Save" {
				saveBtn = v
			}
		}
	})
	if hostEntry == nil || saveBtn == nil {
		t.Fatalf("dialog widgets not found (host=%v save=%v)", hostEntry, saveBtn)
	}
	hostEntry.SetText("api.prod")
	saveBtn.OnTapped()

	env := s.ActiveEnvironment()
	if env == nil {
		t.Fatal("no active environment after save")
	}
	var tok, host *model.Variable
	for i := range env.Variables {
		if env.Variables[i].Value == envSecretMask {
			t.Errorf("the secret mask was persisted as a value: %+v", env.Variables[i])
		}
		switch env.Variables[i].Key {
		case "TOKEN":
			tok = &env.Variables[i]
		case "HOST":
			host = &env.Variables[i]
		}
	}
	if tok == nil || tok.Value != "s3cret-value" || !tok.Secret {
		t.Errorf("un-revealed secret not preserved on save: %+v", tok)
	}
	if host == nil || host.Value != "api.prod" {
		t.Errorf("non-secret edit not saved: %+v", host)
	}
}
