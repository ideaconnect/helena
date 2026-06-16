package ui

import (
	"math"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// treeRowByID returns the live treeRow bound to nodeID via the drag registry.
func treeRowByID(m *MainUI, id string) *treeRow {
	for row, rid := range m.treeRows {
		if rid == id {
			return row
		}
	}
	return nil
}

// firstGlyphAbsX recurses the rendered object tree and returns the absolute X of
// the first *canvas.Text (the rendered glyph origin for a leading text object).
func firstGlyphAbsX(o fyne.CanvasObject) (float32, bool) {
	drv := fyne.CurrentApp().Driver()
	switch v := o.(type) {
	case *canvas.Text:
		return drv.AbsolutePositionForObject(v).X, true
	case *fyne.Container:
		for _, c := range v.Objects {
			if x, ok := firstGlyphAbsX(c); ok {
				return x, true
			}
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			if x, ok := firstGlyphAbsX(c); ok {
				return x, true
			}
		}
	}
	return 0, false
}

// TestTreeRowFolderRequestSameDepthAlignment pins that a request's method chip
// and a same-depth folder's name start at the same x (the #tree-align fix): the
// chip is left-padded by the same inner padding a Label insets its text by, and
// the padding wrapper is hidden for folders so it doesn't shift the folder name.
// Fails before the fix (chip glyph ~innerPadding left of the folder name).
func TestTreeRowFolderRequestSameDepthAlignment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{
		Name:     "C0",
		Folders:  []model.Folder{{Name: "Folder"}},
		Requests: []model.Request{{Name: "Req", Method: model.GET}},
	}
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

	reqRow := treeRowByID(m, "0/r0")    // request at depth 1
	folderRow := treeRowByID(m, "0/f0") // folder at the SAME depth
	if reqRow == nil || folderRow == nil {
		t.Fatalf("rows not bound (req=%v folder=%v, %d rows)", reqRow, folderRow, len(m.treeRows))
	}

	drv := fyne.CurrentApp().Driver()
	chipX := drv.AbsolutePositionForObject(reqRow.method).X
	folderX, ok := firstGlyphAbsX(folderRow.name)
	if !ok {
		t.Fatal("folder name glyph not found")
	}

	if math.Abs(float64(chipX-folderX)) > 0.5 {
		t.Errorf("method chip glyph X=%v misaligned with folder name glyph X=%v (want equal)", chipX, folderX)
	}

	// Vertical-centering guard: the chip box keeps the full row height at y=0, so
	// canvas.Text self-centers exactly as before the fix.
	if got := reqRow.method.Position().Y; got != 0 {
		t.Errorf("chip Y=%v; want 0 (full-height box, vertical centering preserved)", got)
	}
	if got, want := reqRow.method.Size().Height, reqRow.Size().Height; math.Abs(float64(got-want)) > 0.5 {
		t.Errorf("chip height=%v; want full row height %v", got, want)
	}
}
