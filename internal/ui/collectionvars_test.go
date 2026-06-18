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

// TestEditCollectionVariablesSaves pins #80's UI editor: editing a collection
// variable and saving persists it back to the collection's YAML.
func TestEditCollectionVariablesSaves(t *testing.T) {
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{Name: "C0", Variables: []model.Variable{{Enabled: true, Key: "base", Value: "v1"}}}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)

	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(800, 600))
	defer w.Close()
	m.SetWindow(w)

	m.editCollectionVariables()
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("collection variables dialog did not open")
	}

	var valEntry *widget.Entry
	var saveBtn *widget.Button
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Entry:
			if v.Text == "v1" {
				valEntry = v
			}
		case *widget.Button:
			if v.Text == "Save" {
				saveBtn = v
			}
		}
	})
	if valEntry == nil || saveBtn == nil {
		t.Fatalf("dialog widgets not found (val=%v save=%v)", valEntry, saveBtn)
	}
	valEntry.SetText("v2")
	saveBtn.OnTapped()

	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Variables) != 1 || reloaded.Variables[0].Key != "base" || reloaded.Variables[0].Value != "v2" {
		t.Errorf("collection variable not persisted via editor: %+v", reloaded.Variables)
	}
}
