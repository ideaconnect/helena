package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// laidOutDragUI builds a real MainUI over a 3-request collection, lays it out in
// a headless window, and opens every branch so the tree binds (and positions)
// its rows — the setup the drag hit-testing needs.
func laidOutDragUI(t *testing.T) (*MainUI, *session.Session) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{Name: "C0", Requests: []model.Request{
		{Name: "A", Method: model.GET}, {Name: "B", Method: model.GET}, {Name: "Cc", Method: model.GET},
	}}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	test.NewApp()
	s, _ := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(1200, 800))
	m.SetWindow(w)
	m.Tree.OpenAllBranches()
	w.Resize(fyne.NewSize(1200, 800))
	return m, s
}

func (m *MainUI) rowByID(id string) *treeRow {
	for row, rid := range m.treeRows {
		if rid == id {
			return row
		}
	}
	return nil
}

// dragRowOnto drives a full drag of srcID released over targetID at fracY of the
// target row's height (e.g. 0.75 = lower half), through the real hit-testing.
func (m *MainUI) dragRowOnto(t *testing.T, srcID, targetID string, fracY float32) {
	t.Helper()
	tgt := m.rowByID(targetID)
	if m.rowByID(srcID) == nil || tgt == nil {
		t.Fatalf("rows not bound (src=%q tgt=%q, %d rows)", srcID, targetID, len(m.treeRows))
	}
	p := fyne.CurrentApp().Driver().AbsolutePositionForObject(tgt)
	h := tgt.Size().Height
	abs := fyne.NewPos(p.X+5, p.Y+h*fracY)
	m.dragTreeNode(srcID, &fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: abs}})
	m.dropTreeNode(srcID)
}

// TestDragReorderRequestE2E exercises the whole drag pipeline (row registry +
// absolute-position hit-testing + plan + MoveNode) against a laid-out tree.
func TestDragReorderRequestE2E(t *testing.T) {
	m, s := laidOutDragUI(t)

	// Drag A onto the lower half of Cc → A lands after Cc: [B, Cc, A].
	m.dragRowOnto(t, "0/r0", "0/r2", 0.75)
	for id, want := range map[string]string{"0/r0": "B", "0/r1": "Cc", "0/r2": "A"} {
		if got := reqNameUI(s, id); got != want {
			t.Errorf("after reorder %s = %q, want %q", id, got, want)
		}
	}
}

func reqNameUI(s *session.Session, id string) string {
	if r, ok := s.Tree().Request(id); ok {
		return r.Name
	}
	return ""
}
