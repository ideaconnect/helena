package ui

import (
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

func writeSingleReqCollection(t *testing.T, name, reqID string) string {
	t.Helper()
	c := model.Collection{Name: name, Requests: []model.Request{
		{ID: reqID, Name: "Req", Method: model.GET, URL: "https://x"},
	}}
	dir := filepath.Join(t.TempDir(), name)
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

func openSessionWith2Collections(t *testing.T) (*session.Session, string, string) {
	t.Helper()
	test.NewApp()
	// Both collections deliberately share a Request.ID — IDs are unique only
	// within a collection.
	dirA := writeSingleReqCollection(t, "collA", "shared")
	dirB := writeSingleReqCollection(t, "collB", "shared")
	sess, err := session.New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dirA); err != nil {
		t.Fatalf("open A: %v", err)
	}
	if err := sess.OpenCollection(dirB); err != nil {
		t.Fatalf("open B: %v", err)
	}
	return sess, dirA, dirB
}

// TestTabDedupRespectsCollection verifies two collections that share a
// Request.ID open as two distinct tabs, not one (#104).
func TestTabDedupRespectsCollection(t *testing.T) {
	sess, dirA, dirB := openSessionWith2Collections(t)
	m := NewMainUI(sess)

	m.openOrActivate("0/r0") // collA / shared
	ia := m.tabIndexByIdentity(dirA, "shared")
	m.openOrActivate("1/r0") // collB / shared
	ib := m.tabIndexByIdentity(dirB, "shared")

	if ia < 0 || ib < 0 || ia == ib {
		t.Errorf("expected two distinct tabs for (collA,shared) and (collB,shared), got ia=%d ib=%d (tabs=%d)", ia, ib, len(m.tabs))
	}
}

// TestCommitScratchTabDefersActiveSwitchOnFailure verifies a failed scratch
// commit does not switch the active collection (#105).
func TestCommitScratchTabDefersActiveSwitchOnFailure(t *testing.T) {
	sess, _, dirB := openSessionWith2Collections(t)
	sess.SetActiveCollection(1) // dirB active
	m := NewMainUI(sess)

	m.newScratchTab()
	tab := m.activeTab()
	if tab == nil || !tab.scratch {
		t.Fatalf("expected an active scratch tab")
	}
	// Commit into a non-existent folder in collection 0: CollectionIndex is valid
	// (0) but AddRequestValue fails, so the active collection must stay dirB.
	m.commitScratchTab(tab, "0/f9", "X")

	if got := sess.ActiveCollectionDir(); got != dirB {
		t.Errorf("failed commit switched active collection to %q, want %q (unchanged)", got, dirB)
	}
	if !tab.scratch {
		t.Error("failed commit cleared the scratch flag")
	}
}

// TestChainRefSuggestionsExcludeByIdentity verifies the self-exclusion is by
// request identity, so a same-named request in another folder stays
// suggestable (#107).
func TestChainRefSuggestionsExcludeByIdentity(t *testing.T) {
	test.NewApp()
	c := model.Collection{Name: "C", Folders: []model.Folder{
		{Name: "A", Requests: []model.Request{{ID: "idA", Name: "Login", Method: model.GET, URL: "x"}}},
		{Name: "B", Requests: []model.Request{{ID: "idB", Name: "Login", Method: model.GET, URL: "x"}}},
	}}
	dir := filepath.Join(t.TempDir(), "c")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	sess, err := session.New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	m := NewMainUI(sess)
	reqA, ok := sess.Tree().Request("0/f0/r0")
	if !ok {
		t.Fatalf("could not load A/Login")
	}
	m.currentRequest = reqA
	m.currentRequestID = "idA"

	sugg := m.chainRefSuggestions()
	if slices.Contains(sugg, "A/Login") {
		t.Errorf("suggestions include the request being edited: %v", sugg)
	}
	if !slices.Contains(sugg, "B/Login") {
		t.Errorf("suggestions exclude a same-named request in another folder: %v", sugg)
	}
}
