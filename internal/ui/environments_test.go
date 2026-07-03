package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestEnvironmentsDialogGatesActions pins that the environments manager's
// Rename/Delete/Set-active icon buttons are disabled until a row is selected,
// matching the Workspaces dialog. The four action buttons are the only tooltip
// (ttwidget) buttons in the dialog overlay.
func TestEnvironmentsDialogGatesActions(t *testing.T) {
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

	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(700, 500))
	defer w.Close()
	m.SetWindow(w)

	m.manageEnvironments()
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("environments dialog did not open")
	}

	var buttons []*ttwidget.Button
	var list *widget.List
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ttwidget.Button:
			buttons = append(buttons, v)
		case *widget.List:
			if list == nil {
				list = v
			}
		}
	})
	if len(buttons) != 4 {
		t.Fatalf("expected 4 action buttons (New/Rename/Delete/Set-active), got %d", len(buttons))
	}
	if list == nil {
		t.Fatal("environment list not found")
	}

	enabled := func() int {
		n := 0
		for _, b := range buttons {
			if !b.Disabled() {
				n++
			}
		}
		return n
	}
	if got := enabled(); got != 1 { // only New enabled with nothing selected
		t.Errorf("with no selection, %d of 4 buttons enabled; want 1 (New only)", got)
	}
	list.Select(0)
	if got := enabled(); got != 4 {
		t.Errorf("after selecting an environment, %d of 4 buttons enabled; want 4", got)
	}
	list.Unselect(0)
	if got := enabled(); got != 1 {
		t.Errorf("after unselecting, %d of 4 buttons enabled; want 1", got)
	}
}

// TestStartupPreservesPersistedEnvSelection is a launch-to-launch regression
// test: the Environment dropdown's construction-time seeding used to fire
// onEnvChanged, whose SetActiveEnv("") deleted the persisted per-collection
// environment choice before refreshEnvironments could restore it — resetting
// the user's selection on every launch.
func TestStartupPreservesPersistedEnvSelection(t *testing.T) {
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	if err := storage.Save(model.Collection{Name: "C0"}, dir); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "cfg.yml")
	s, err := session.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	if err := s.AddEnvironment("Staging"); err != nil {
		t.Fatal(err)
	}
	s.SetActiveEnv("Staging")

	// Relaunch: a fresh session + UI must still see Staging as active.
	s2, err := session.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMainUI(s2)
	if got := s2.ActiveEnvName(); got != "Staging" {
		t.Fatalf("ActiveEnvName after UI construction = %q, want Staging", got)
	}
	if m.Environment.Selected != "Staging" {
		t.Errorf("Environment dropdown = %q, want Staging", m.Environment.Selected)
	}
}
