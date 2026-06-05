package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// writeTabTestCollection writes a collection with two sibling root requests
// (First 0/r0, Second 0/r1) and a nested one (Sub/Third 0/f0/r0), so the tab
// tests can exercise dedup, sibling-delete remapping, and nested opens.
func writeTabTestCollection(t *testing.T) string {
	t.Helper()
	// Explicit IDs so the on-disk YAML carries them (as a collection Helena has
	// saved would), giving tab restoration a stable anchor across sessions.
	c := model.Collection{
		Name: "Tabs API",
		Requests: []model.Request{
			{ID: "id-first", Name: "First", Method: model.GET, URL: "https://x/first"},
			{ID: "id-second", Name: "Second", Method: model.POST, URL: "https://x/second"},
		},
		Folders: []model.Folder{{
			Name:     "Sub",
			Requests: []model.Request{{ID: "id-third", Name: "Third", Method: model.GET, URL: "https://x/third"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "tabs-api")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

// newTabUI builds a MainUI over a freshly-opened collection in a headless app.
func newTabUI(t *testing.T) (*MainUI, *session.Session, string) {
	t.Helper()
	test.NewApp()
	dir := writeTabTestCollection(t)
	sess, err := session.New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	return NewMainUI(sess), sess, dir
}

// TestOpenOrActivateDedups verifies opening a request twice reuses its tab, and
// opening a different request adds a second tab.
func TestOpenOrActivateDedups(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	if len(m.tabs) != 1 {
		t.Fatalf("after first open: %d tabs, want 1", len(m.tabs))
	}
	m.openOrActivate("0/f0/r0")
	if len(m.tabs) != 2 {
		t.Fatalf("after second open: %d tabs, want 2", len(m.tabs))
	}
	// Re-opening the first request activates its existing tab, no duplicate.
	m.openOrActivate("0/r0")
	if len(m.tabs) != 2 {
		t.Errorf("re-open duplicated: %d tabs, want 2", len(m.tabs))
	}
	if m.activeTabIdx != 0 {
		t.Errorf("activeTabIdx = %d, want 0 (first request's tab)", m.activeTabIdx)
	}
	if m.currentRequestID != "0/r0" {
		t.Errorf("currentRequestID = %q, want 0/r0", m.currentRequestID)
	}
}

// TestPerTabResponseRestore verifies each tab keeps its own response: switching
// away clears the panel, switching back restores that tab's body.
func TestPerTabResponseRestore(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 (active)

	m.activateTab(0)
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: "FIRST-BODY", status: "200 OK"})
	if string(m.pv.Source()) != "FIRST-BODY" {
		t.Fatalf("after deliver to tab0: raw = %q, want FIRST-BODY", string(m.pv.Source()))
	}

	m.activateTab(1) // tab1 has no response → panel clears
	if string(m.pv.Source()) != "" {
		t.Errorf("tab1 raw = %q, want empty", string(m.pv.Source()))
	}
	m.deliverResponse(m.tabs[1], &tabResponse{rawBody: "SECOND-BODY", status: "201"})

	m.activateTab(0) // back to tab0 → its body restored
	if string(m.pv.Source()) != "FIRST-BODY" {
		t.Errorf("tab0 restored raw = %q, want FIRST-BODY", string(m.pv.Source()))
	}
	m.activateTab(1)
	if string(m.pv.Source()) != "SECOND-BODY" {
		t.Errorf("tab1 restored raw = %q, want SECOND-BODY", string(m.pv.Source()))
	}
}

// TestDeliverResponseToInactiveTabDoesNotRepaint verifies a Send that completes
// after the user switched tabs stores its result on the originating tab without
// disturbing the now-visible tab.
func TestDeliverResponseToInactiveTabDoesNotRepaint(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 active
	initTab := m.tabs[0]

	m.activateTab(1) // user switched to tab 1 mid-Send
	m.deliverResponse(initTab, &tabResponse{rawBody: "LATE", status: "200"})

	if string(m.pv.Source()) == "LATE" {
		t.Error("inactive tab's response repainted the active panel")
	}
	if initTab.resp == nil || initTab.resp.rawBody != "LATE" {
		t.Error("response not stored on the originating tab")
	}
	m.activateTab(0)
	if string(m.pv.Source()) != "LATE" {
		t.Errorf("originating tab raw after switch back = %q, want LATE", string(m.pv.Source()))
	}
}

// TestScratchTabSaveAs verifies the "+" scratch tab binds an unsaved request
// (no node ID), and committing it writes a tree-backed request and rebinds the
// tab.
func TestScratchTabSaveAs(t *testing.T) {
	m, _, dir := newTabUI(t)
	m.newScratchTab()
	if len(m.tabs) != 1 || !m.tabs[0].scratch {
		t.Fatalf("newScratchTab: tabs=%d scratch=%v", len(m.tabs), m.tabs[0].scratch)
	}
	if m.currentRequest == nil || m.currentRequestID != "" {
		t.Fatalf("scratch active: currentRequest=%v currentRequestID=%q (want non-nil, empty)",
			m.currentRequest, m.currentRequestID)
	}
	// Edit the scratch request, then save it into the collection root.
	m.currentRequest.URL = "https://x/new"
	m.currentRequest.Method = model.PUT
	m.commitScratchTab(m.tabs[0], "0", "Saved Scratch")

	tab := m.tabs[0]
	if tab.scratch || tab.scratchReq != nil {
		t.Errorf("tab still scratch after commit: %+v", tab)
	}
	if m.currentRequestID == "" {
		t.Error("currentRequestID still empty after commit, want a node ID")
	}
	if tab.collection != dir {
		t.Errorf("tab collection = %q, want %q", tab.collection, dir)
	}
	saved, ok := m.sess.Tree().Request(tab.nodeID)
	if !ok || saved.Name != "Saved Scratch" || saved.URL != "https://x/new" || saved.Method != model.PUT {
		t.Errorf("saved request = %+v ok=%v", saved, ok)
	}
}

// TestReconcileAfterDeleteDropsAndRemaps verifies that deleting an open
// request drops its tab and remaps a surviving sibling tab's node ID.
func TestReconcileAfterDeleteDropsAndRemaps(t *testing.T) {
	m, sess, _ := newTabUI(t)
	m.openOrActivate("0/r0") // First, tab 0
	m.openOrActivate("0/r1") // Second, tab 1 (active)

	if err := sess.DeleteItem("0/r0"); err != nil { // remove First; Second shifts to 0/r0
		t.Fatalf("DeleteItem: %v", err)
	}
	m.reconcileTabs()

	if len(m.tabs) != 1 {
		t.Fatalf("after delete: %d tabs, want 1", len(m.tabs))
	}
	if m.tabs[0].nodeID != "0/r0" {
		t.Errorf("surviving tab node ID = %q, want remapped 0/r0", m.tabs[0].nodeID)
	}
	if m.currentRequestID != "0/r0" || m.currentRequest == nil || m.currentRequest.Name != "Second" {
		t.Errorf("active binding after reconcile = %q %+v, want Second at 0/r0",
			m.currentRequestID, m.currentRequest)
	}
}

// TestCloseTabActivatesNeighborThenClears verifies closing the active tab falls
// to a neighbor, and closing the last tab clears the editor.
func TestCloseTabActivatesNeighborThenClears(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.openOrActivate("0/r1") // active = tab 1

	m.closeTab(m.tabs[1])
	if len(m.tabs) != 1 || m.activeTabIdx != 0 {
		t.Fatalf("after close active: tabs=%d active=%d, want 1/0", len(m.tabs), m.activeTabIdx)
	}
	m.closeTab(m.tabs[0])
	if len(m.tabs) != 0 || m.activeTabIdx != -1 {
		t.Fatalf("after close last: tabs=%d active=%d, want 0/-1", len(m.tabs), m.activeTabIdx)
	}
	if m.currentRequest != nil {
		t.Error("currentRequest not cleared after closing last tab")
	}
}

// TestRestoreTabsOnLaunch verifies persisted tabs reopen (with the active one
// focused) when a new MainUI is built over the same config.
func TestRestoreTabsOnLaunch(t *testing.T) {
	test.NewApp()
	dir := writeTabTestCollection(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s1, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := s1.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	first, _ := s1.Tree().Request("0/r0")
	second, _ := s1.Tree().Request("0/r1")
	s1.SetOpenTabs([]config.UIOpenTab{
		{Collection: dir, RequestID: first.ID},
		{Collection: dir, RequestID: second.ID},
	}, 1)

	// Fresh session over the same config auto-loads the collection (it is in
	// the workspace) and NewMainUI restores the tabs.
	s2, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New (reopen): %v", err)
	}
	m := NewMainUI(s2)
	if len(m.tabs) != 2 {
		t.Fatalf("restored %d tabs, want 2", len(m.tabs))
	}
	if m.activeTabIdx != 1 {
		t.Errorf("restored activeTabIdx = %d, want 1", m.activeTabIdx)
	}
	if m.currentRequestID != "0/r1" {
		t.Errorf("active request = %q, want 0/r1 (Second)", m.currentRequestID)
	}
}

// TestDropIndex verifies the insertion index is the count of tab centers left
// of the pointer.
func TestDropIndex(t *testing.T) {
	centers := []float32{10, 30, 50} // three other tabs
	cases := []struct {
		px   float32
		want int
	}{
		{0, 0},  // before all
		{20, 1}, // between first and second
		{40, 2}, // between second and third
		{99, 3}, // after all
		{30, 1}, // exactly on a center → not strictly greater
	}
	for _, c := range cases {
		if got := dropIndex(centers, c.px); got != c.want {
			t.Errorf("dropIndex(%v) = %d, want %d", c.px, got, c.want)
		}
	}
}

// TestMoveTabModel verifies the pure reorder lands the dragged tab at the
// requested insertion index among the others, with clamping.
func TestMoveTabModel(t *testing.T) {
	a, b, c := &openTab{requestID: "a"}, &openTab{requestID: "b"}, &openTab{requestID: "c"}
	tabs := []*openTab{a, b, c}
	ids := func(ts []*openTab) string {
		s := ""
		for _, x := range ts {
			s += x.requestID
		}
		return s
	}
	if got := ids(moveTabModel(tabs, a, 2)); got != "bca" { // a to the end
		t.Errorf("move a→2 = %q, want bca", got)
	}
	if got := ids(moveTabModel(tabs, c, 0)); got != "cab" { // c to the front
		t.Errorf("move c→0 = %q, want cab", got)
	}
	if got := ids(moveTabModel(tabs, b, 99)); got != "acb" { // clamp past end
		t.Errorf("move b→99 = %q, want acb", got)
	}
}

// TestApplyDragTargetReordersAndKeepsActive verifies a drag reorder moves the
// tab and keeps the originally-active tab active.
func TestApplyDragTargetReordersAndKeepsActive(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")    // First, tab 0
	m.openOrActivate("0/r1")    // Second, tab 1
	m.openOrActivate("0/f0/r0") // Third, tab 2 (active)
	first := m.tabs[0]

	m.applyDragTarget(first, 2) // drag First to the end
	if m.tabs[2] != first {
		t.Error("First not at the end after drag")
	}
	// The active tab (Third) stays active across the reorder.
	if _, name := m.tabLabel(m.activeTab()); name != "Third" {
		t.Errorf("active tab = %q, want Third (stayed active across reorder)", name)
	}
}

// TestDragEndPersistsOrder verifies that after a reorder, dragEnd writes the new
// order to the session.
func TestDragEndPersistsOrder(t *testing.T) {
	m, sess, dir := newTabUI(t)
	m.openOrActivate("0/r0") // First
	m.openOrActivate("0/r1") // Second
	first := m.tabs[0]

	m.applyDragTarget(first, 1) // First → after Second: [Second, First]
	m.dragEnd()

	tabs, _ := sess.OpenTabs()
	if len(tabs) != 2 || tabs[0].Collection != dir {
		t.Fatalf("persisted tabs = %+v", tabs)
	}
	second, _ := sess.Tree().Request("0/r1")
	if tabs[0].RequestID != second.ID {
		t.Errorf("persisted order not reordered: first persisted = %q, want Second", tabs[0].RequestID)
	}
}

// TestTabMenuItems verifies the overflow menu lists every tab, checks the
// active one, and that selecting an item activates that tab.
func TestTabMenuItems(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0") // First, tab 0
	m.openOrActivate("0/r1") // Second, tab 1 (active)

	items := m.tabMenuItems()
	if len(items) != 2 {
		t.Fatalf("menu items = %d, want 2", len(items))
	}
	if items[0].Label != "GET  First" || items[1].Label != "POST  Second" {
		t.Errorf("menu labels = %q / %q", items[0].Label, items[1].Label)
	}
	if items[0].Checked || !items[1].Checked {
		t.Errorf("checked = %v/%v, want false/true (Second active)", items[0].Checked, items[1].Checked)
	}
	// Selecting the first item activates tab 0.
	items[0].Action()
	if m.activeTabIdx != 0 {
		t.Errorf("after selecting item 0, activeTabIdx = %d, want 0", m.activeTabIdx)
	}
}

// TestNewScratchTabBindsBlankRequest verifies the "+" affordance opens a blank
// GET with no node ID and that closing it (no content) needs no confirmation.
func TestNewScratchTabBindsBlankRequest(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.newScratchTab()
	if m.currentRequest.Method != model.GET || m.currentRequest.URL != "" {
		t.Errorf("scratch request = %+v, want blank GET", m.currentRequest)
	}
	// Empty scratch → requestCloseTab closes without a dialog (no content).
	m.requestCloseTab(m.tabs[0])
	if len(m.tabs) != 0 {
		t.Errorf("empty scratch not closed: %d tabs", len(m.tabs))
	}
}
