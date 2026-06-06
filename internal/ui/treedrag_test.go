package ui

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// dragSession builds a session with two collections. Collection 0:
// root requests A(0/r0), B(0/r1); folders F(0/f0){X(0/f0/r0)}, G(0/f1).
func dragSession(t *testing.T) *session.Session {
	t.Helper()
	col := model.Collection{
		Name:     "C0",
		Requests: []model.Request{{Name: "A", Method: model.GET}, {Name: "B", Method: model.GET}},
		Folders: []model.Folder{
			{Name: "F", Requests: []model.Request{{Name: "X", Method: model.GET}}},
			{Name: "G"},
		},
	}
	d0 := filepath.Join(t.TempDir(), "c0")
	if err := storage.Save(col, d0); err != nil {
		t.Fatal(err)
	}
	d1 := filepath.Join(t.TempDir(), "c1")
	if err := storage.Save(model.Collection{Name: "C1"}, d1); err != nil {
		t.Fatal(err)
	}
	s, err := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(d0); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(d1); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPlanNodeDrop(t *testing.T) {
	m := &MainUI{sess: dragSession(t)}

	// Request onto request, top half → sibling before (index = target index).
	if p := m.planNodeDrop("0/r0", "0/r1", true, 0, 0); !p.valid || p.into || p.dstParent != "0" || p.index != 1 {
		t.Errorf("request-before-request = %+v", p)
	}
	// Request onto request, bottom half → after (index+1).
	if p := m.planNodeDrop("0/r0", "0/r1", false, 0, 0); p.index != 2 {
		t.Errorf("request-after-request index = %d, want 2", p.index)
	}
	// Request onto folder, bottom half → into folder (append).
	if p := m.planNodeDrop("0/r0", "0/f0", false, 0, 0); !p.valid || !p.into || p.dstParent != "0/f0" {
		t.Errorf("request-into-folder = %+v", p)
	}
	// Folder before folder (same slice) → sibling at the target's index.
	if p := m.planNodeDrop("0/f1", "0/f0", true, 0, 0); !p.valid || p.into || p.dstParent != "0" || p.index != 0 {
		t.Errorf("folder-before-folder = %+v", p)
	}
	// Folder onto a collection root's top half → the root's first folder
	// (dstParent is the root id, not splitNode's empty parent).
	if p := m.planNodeDrop("0/f1", "0", true, 0, 0); !p.valid || p.dstParent != "0" || p.index != 0 {
		t.Errorf("folder-onto-collection-root-top = %+v", p)
	}
	// Reject dropping a folder onto itself / into a descendant.
	if p := m.planNodeDrop("0/f0", "0/f0", true, 0, 0); p.valid {
		t.Error("folder onto itself should be invalid")
	}
	if p := m.planNodeDrop("0/f0", "0/f0/r0", false, 0, 0); p.valid {
		t.Error("folder into its descendant should be invalid")
	}
	// Cross-collection is rejected.
	if p := m.planNodeDrop("0/r0", "1", false, 0, 0); p.valid {
		t.Error("cross-collection node move should be invalid")
	}
}

func TestPlanCollectionDrop(t *testing.T) {
	m := &MainUI{sess: dragSession(t)}

	// Drag collection 0 onto collection 1, bottom half → final index 1.
	if p := m.planCollectionDrop("0", "1", false, 0, 0); !p.valid || !p.isCollection || p.fromCol != 0 || p.toCol != 1 {
		t.Errorf("collection 0→after 1 = %+v", p)
	}
	// Drag collection 1 onto collection 0, top half → final index 0.
	if p := m.planCollectionDrop("1", "0", true, 0, 0); p.fromCol != 1 || p.toCol != 0 {
		t.Errorf("collection 1→before 0 = %+v", p)
	}
	// Same collection → invalid.
	if p := m.planCollectionDrop("0", "0", true, 0, 0); p.valid {
		t.Error("collection onto itself should be invalid")
	}
}

func TestNodeIDHelpers(t *testing.T) {
	if !isCollectionID("0") || isCollectionID("0/r1") {
		t.Error("isCollectionID")
	}
	if collectionIndexOf("2/f1/r0") != 2 || collectionIndexOf("x") != -1 {
		t.Error("collectionIndexOf")
	}
	for id, want := range map[string]string{"0": "c", "0/f1": "f", "0/f1/r2": "r"} {
		if got := nodeKind(id); got != want {
			t.Errorf("nodeKind(%q) = %q, want %q", id, got, want)
		}
	}
	if p, idx, ok := splitNode("0/f1/r2"); !ok || p != "0/f1" || idx != 2 {
		t.Errorf("splitNode = (%q,%d,%v)", p, idx, ok)
	}
}
