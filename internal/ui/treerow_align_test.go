package ui

import (
	"math"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

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

// TestTreeRowChipInArrowColumn pins that a request's method chip sits in the
// disclosure-arrow column — left of the content origin by iconSize+pad — so it
// lines up with same-depth folders' arrows rather than their name text (the
// requested behaviour). It also confirms the chip is clearly LEFT of a
// same-depth folder's name glyph (i.e. not in the text column).
func TestTreeRowChipInArrowColumn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{
		Name:     "C0",
		Folders:  []model.Folder{{Name: "Folder"}},
		Requests: []model.Request{{Name: "Req", Method: model.GET}},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}

	a := test.NewApp()
	ApplyTheme(a, model.ThemeDark) // install the real theme stack (sidebarTheme sizes)
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
	contentX := drv.AbsolutePositionForObject(reqRow).X
	folderContentX := drv.AbsolutePositionForObject(folderRow).X
	if math.Abs(float64(contentX-folderContentX)) > 0.5 {
		t.Fatalf("rows not at same depth: req contentX=%v folder contentX=%v", contentX, folderContentX)
	}

	// The tree paints the disclosure arrow iconSize+pad to the left of the
	// content origin; the chip must land in that same column.
	arrowGap := theme.SizeForWidget(theme.SizeNameInlineIcon, reqRow) +
		theme.SizeForWidget(theme.SizeNamePadding, reqRow)
	chipX := drv.AbsolutePositionForObject(reqRow.method).X
	if want := contentX - arrowGap; math.Abs(float64(chipX-want)) > 0.5 {
		t.Errorf("chip X=%v; want arrow column %v (contentX %v - arrowGap %v)", chipX, want, contentX, arrowGap)
	}

	// Sanity: the chip is clearly LEFT of the folder's name glyph (the text
	// column), i.e. not re-aligned with the name like the previous behaviour.
	folderNameX, ok := firstGlyphAbsX(folderRow.name)
	if !ok {
		t.Fatal("folder name glyph not found")
	}
	if chipX >= folderNameX {
		t.Errorf("chip X=%v not left of folder name glyph X=%v (chip should be in the arrow column)", chipX, folderNameX)
	}

	// Vertical-centering guard: the chip box keeps the full row height at y=0, so
	// canvas.Text self-centers exactly as before.
	if got := reqRow.method.Position().Y; got != 0 {
		t.Errorf("chip Y=%v; want 0 (full-height box, vertical centering preserved)", got)
	}
	if got, want := reqRow.method.Size().Height, reqRow.Size().Height; math.Abs(float64(got-want)) > 0.5 {
		t.Errorf("chip height=%v; want full row height %v", got, want)
	}
}
