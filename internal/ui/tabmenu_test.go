package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/storage"
)

// openThreeTabs opens First / Second / Third as tabs 0..2, leaving 2 active.
func openThreeTabs(t *testing.T, m *MainUI) {
	t.Helper()
	m.openOrActivate("0/r0")
	m.openOrActivate("0/r1")
	m.openOrActivate("0/f0/r0")
	if len(m.tabs) != 3 || m.activeTabIdx != 2 {
		t.Fatalf("setup: tabs=%d active=%d, want 3/2", len(m.tabs), m.activeTabIdx)
	}
}

// menuLabels returns the non-separator labels of a menu, "!" prefixed when disabled.
func menuLabels(items []*fyne.MenuItem) []string {
	var out []string
	for _, it := range items {
		if it.IsSeparator {
			continue
		}
		l := it.Label
		if it.Disabled {
			l = "!" + l
		}
		out = append(out, l)
	}
	return out
}

// TestTabContextMenuItems pins the menu's entries and which are disabled per
// position: Close Others needs a second tab, Close to the Right a tab after.
func TestTabContextMenuItems(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	got := menuLabels(m.tabContextMenuItems(m.tabs[0]))
	want := []string{"Save", "Close", "!Close Others", "!Close to the Right"}
	if !equalStrings(got, want) {
		t.Errorf("lone tab menu = %v, want %v", got, want)
	}

	openThreeTabs(t, m)
	cases := []struct {
		idx  int
		want []string
	}{
		{0, []string{"Save", "Close", "Close Others", "Close to the Right"}},
		{1, []string{"Save", "Close", "Close Others", "Close to the Right"}},
		{2, []string{"Save", "Close", "Close Others", "!Close to the Right"}},
	}
	for _, c := range cases {
		if got := menuLabels(m.tabContextMenuItems(m.tabs[c.idx])); !equalStrings(got, c.want) {
			t.Errorf("tab %d menu = %v, want %v", c.idx, got, c.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCloseToTheRightHandsOverToAnchor: closing the tabs right of a non-active
// tab drops the active one, so the anchor becomes active and the editor
// rebinds to it.
func TestCloseToTheRightHandsOverToAnchor(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	anchor := m.tabs[0]
	m.tabContextMenuItems(anchor)[4].Action() // Close to the Right
	if len(m.tabs) != 1 || m.tabs[0] != anchor || m.activeTabIdx != 0 {
		t.Fatalf("after close-right: tabs=%d active=%d anchorKept=%v", len(m.tabs), m.activeTabIdx, len(m.tabs) > 0 && m.tabs[0] == anchor)
	}
	if m.currentRequest == nil || m.currentRequest.Name != "First" {
		t.Errorf("editor not rebound to the anchor: %+v", m.currentRequest)
	}
}

// TestCloseToTheRightKeepsActiveLeftOfAnchor: when the active tab sits left
// of the anchor it survives untouched — no rebind, index unchanged.
func TestCloseToTheRightKeepsActiveLeftOfAnchor(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	m.activateTab(0)
	before := m.currentRequest
	m.closeTabs(m.tabsRightOf(m.tabs[1]), m.tabs[1])
	if len(m.tabs) != 2 || m.activeTabIdx != 0 || m.currentRequest != before {
		t.Errorf("tabs=%d active=%d rebound=%v, want 2/0/false", len(m.tabs), m.activeTabIdx, m.currentRequest != before)
	}
}

// TestCloseOthersFromActiveTabShiftsIndex: Close Others on the active
// (rightmost) tab keeps it bound and moves its index to 0.
func TestCloseOthersFromActiveTabShiftsIndex(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	anchor := m.tabs[2]
	before := m.currentRequest
	m.tabContextMenuItems(anchor)[3].Action() // Close Others
	if len(m.tabs) != 1 || m.tabs[0] != anchor || m.activeTabIdx != 0 {
		t.Fatalf("after close-others: tabs=%d active=%d", len(m.tabs), m.activeTabIdx)
	}
	if m.currentRequest != before {
		t.Error("active tab was rebound although it survived")
	}
}

// TestCloseOthersFromInactiveTabActivatesIt: Close Others on a non-active tab
// closes the active one and activates the anchor.
func TestCloseOthersFromInactiveTabActivatesIt(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	anchor := m.tabs[1]
	m.tabContextMenuItems(anchor)[3].Action() // Close Others
	if len(m.tabs) != 1 || m.tabs[0] != anchor || m.activeTabIdx != 0 {
		t.Fatalf("after close-others: tabs=%d active=%d", len(m.tabs), m.activeTabIdx)
	}
	if m.currentRequest == nil || m.currentRequest.Name != "Second" {
		t.Errorf("editor not rebound to the anchor: %+v", m.currentRequest)
	}
}

// TestCloseTabsPersistsSurvivors: the bulk close writes the surviving tab set
// so a relaunch reopens only what is left.
func TestCloseTabsPersistsSurvivors(t *testing.T) {
	m, sess, dir := newTabUI(t)
	openThreeTabs(t, m)
	m.closeTabs(m.tabsOtherThan(m.tabs[1]), m.tabs[1])
	got, active := sess.OpenTabs()
	if len(got) != 1 || got[0].RequestID != "id-second" || got[0].Collection != dir || active != 0 {
		t.Errorf("persisted tabs = %+v active=%d, want only id-second in %s at 0", got, active, dir)
	}
}

// TestCloseTabsIgnoresStaleAndAnchor: victims already closed are skipped, the
// anchor is never closed even when listed, and an unopened anchor is a no-op.
func TestCloseTabsIgnoresStaleAndAnchor(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	stale := m.tabs[2]
	m.closeTab(stale)
	anchor := m.tabs[0]
	m.closeTabs([]*openTab{stale, anchor}, anchor)
	if len(m.tabs) != 2 {
		t.Errorf("stale/anchor victims changed the strip: %d tabs", len(m.tabs))
	}
	m.closeTabs(m.tabs, stale) // stale anchor
	if len(m.tabs) != 2 {
		t.Errorf("closed-anchor bulk close changed the strip: %d tabs", len(m.tabs))
	}
}

// TestRequestCloseTabsConfirmsScratchContent: a bulk close that would drop a
// scratch tab with content asks first; Cancel keeps every tab, the confirm
// closes them all.
func TestRequestCloseTabsConfirmsScratchContent(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.newScratchTab()
	m.currentRequest.URL = "https://x/unsaved"
	m.openOrActivate("0/r1") // tabs: First, scratch*, Second (active)
	anchor := m.tabs[0]

	m.requestCloseTabs(m.tabsOtherThan(anchor), anchor)
	if len(m.tabs) != 3 {
		t.Fatalf("closed before confirming: %d tabs", len(m.tabs))
	}
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no confirm dialog on screen")
	}
	test.Tap(buttonByText(top, "No"))
	if len(m.tabs) != 3 {
		t.Fatalf("No closed tabs: %d left", len(m.tabs))
	}

	m.requestCloseTabs(m.tabsOtherThan(anchor), anchor)
	top = w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no confirm dialog on the second ask")
	}
	test.Tap(buttonByText(top, "Yes"))
	if len(m.tabs) != 1 || m.tabs[0] != anchor || m.activeTabIdx != 0 {
		t.Errorf("after Yes: tabs=%d active=%d", len(m.tabs), m.activeTabIdx)
	}
}

// TestRequestCloseTabsSilentWithoutScratchContent: tree-backed tabs (and an
// empty scratch) close without a dialog.
func TestRequestCloseTabsSilentWithoutScratchContent(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited-in-tree" // tree-backed edits are not confirmed
	m.newScratchTab()                                 // empty scratch
	m.openOrActivate("0/r1")
	anchor := m.tabs[2]
	m.requestCloseTabs(m.tabsOtherThan(anchor), anchor)
	if top := w.Canvas().Overlays().Top(); top != nil {
		t.Fatal("unexpected confirm dialog")
	}
	if len(m.tabs) != 1 || m.tabs[0] != anchor {
		t.Errorf("tabs=%d, want just the anchor", len(m.tabs))
	}
}

// TestSaveTabActivatesThenSaves: Save on a non-active tab switches to it and
// writes its edits to disk.
func TestSaveTabActivatesThenSaves(t *testing.T) {
	m, _, dir := newTabUI(t)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/first-edited"
	m.openOrActivate("0/r1") // active = Second
	m.saveTab(m.tabs[0])
	if m.activeTabIdx != 0 || m.currentRequest == nil || m.currentRequest.Name != "First" {
		t.Fatalf("saveTab did not activate the tab: active=%d req=%+v", m.activeTabIdx, m.currentRequest)
	}
	if m.Status.Text != "Saved: First" {
		t.Errorf("status = %q, want %q", m.Status.Text, "Saved: First")
	}
	col, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if col.Requests[0].URL != "https://x/first-edited" {
		t.Errorf("on-disk URL = %q, want the edit", col.Requests[0].URL)
	}
	if m.isTabDirty(m.tabs[0]) {
		t.Error("tab still dirty after save")
	}
}

// TestSaveTabOnActiveTabDoesNotRebind: Save on the active tab saves in place.
func TestSaveTabOnActiveTabDoesNotRebind(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.openOrActivate("0/r1")
	before := m.currentRequest
	m.tabContextMenuItems(m.tabs[1])[0].Action() // Save
	if m.currentRequest != before || m.activeTabIdx != 1 {
		t.Errorf("active tab rebound on save: active=%d", m.activeTabIdx)
	}
	if m.Status.Text != "Saved: Second" {
		t.Errorf("status = %q", m.Status.Text)
	}
	m.saveTab(&openTab{requestID: "not-open"}) // unknown tab: no-op
	if m.activeTabIdx != 1 {
		t.Errorf("unknown tab changed the active index to %d", m.activeTabIdx)
	}
}

// TestSaveTabScratchRoutesToSaveAs: Save on a scratch tab opens the Save As
// dialog rather than writing anything.
func TestSaveTabScratchRoutesToSaveAs(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.newScratchTab()
	m.openOrActivate("0/r0") // back to First; scratch is tab 1, inactive
	m.saveTab(m.tabs[1])
	if m.activeTabIdx != 1 {
		t.Fatalf("scratch tab not activated: active=%d", m.activeTabIdx)
	}
	if w.Canvas().Overlays().Top() == nil {
		t.Error("Save As dialog did not open for the scratch tab")
	}
}

// TestRequestTabSecondaryTapOpensMenu: a right-click on a tab widget pops the
// context menu up on the canvas, with the tab's items.
func TestRequestTabSecondaryTapOpensMenu(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(900, 600))
	defer w.Close()
	m.SetWindow(w)
	openThreeTabs(t, m)
	rt := m.tabWidgets[m.tabs[0]]
	if rt == nil {
		t.Fatal("no widget pooled for tab 0")
	}
	test.TapSecondaryAt(rt, fyne.NewPos(4, 4))
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no popup on screen after a secondary tap")
	}
	// The popup menu sits inside an overlay container and its item widgets are
	// unexported; their labels render as canvas.Text, so read those back.
	var got []string
	walkObjects(top, func(o fyne.CanvasObject) {
		if txt, ok := o.(*canvas.Text); ok && txt.Text != "" {
			got = append(got, txt.Text)
		}
	})
	want := []string{"Save", "Close", "Close Others", "Close to the Right"}
	if !equalStrings(got, want) {
		t.Errorf("popup items = %v, want %v", got, want)
	}
	w.Canvas().Overlays().Remove(top)

	// A tab that has left the strip (stale widget callback) opens nothing.
	stale := m.tabs[2]
	m.closeTab(stale)
	m.showTabContextMenu(stale, &fyne.PointEvent{})
	if w.Canvas().Overlays().Top() != nil {
		t.Error("stale tab opened a menu")
	}
}

// TestTabContextMenuCloseItem: the menu's Close entry routes through the
// same confirm-aware path as the tab's close button.
func TestTabContextMenuCloseItem(t *testing.T) {
	m, _, _ := newTabUI(t)
	openThreeTabs(t, m)
	m.tabContextMenuItems(m.tabs[1])[2].Action() // Close
	if len(m.tabs) != 2 || m.activeTabIdx != 1 || m.currentRequest.Name != "Third" {
		t.Errorf("after Close: tabs=%d active=%d req=%q", len(m.tabs), m.activeTabIdx, m.currentRequest.Name)
	}
}

// TestSaveTabVanishedRequest: Save on a tab whose request is gone lets
// activateTab drop the tab and saves nothing.
func TestSaveTabVanishedRequest(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.openOrActivate("0/r1")
	gone := m.tabs[0]
	gone.collection = "/nonexistent"
	m.Status.SetText("")
	m.saveTab(gone)
	if len(m.tabs) != 1 || m.tabs[0] == gone {
		t.Fatalf("vanished tab not dropped: %d tabs", len(m.tabs))
	}
	// Without the post-activate guard, activateTab's close would hand over to
	// Second and saveRequest would then write *that* — so no "Saved:" at all.
	if strings.HasPrefix(m.Status.Text, "Saved:") {
		t.Errorf("saved another request after the target vanished: %q", m.Status.Text)
	}
	if m.Status.Text != "Loaded: Second" {
		t.Errorf("status = %q, want the neighbour's load status", m.Status.Text)
	}
}

// TestShowTabContextMenuHeadless: with no window set, the menu still opens
// without panicking. (The test driver's CanvasForObject always answers with
// its dummy window's canvas, so the nil-canvas guard in showTabContextMenu is
// defensive and unreachable here.)
func TestShowTabContextMenuHeadless(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	if m.tabWidgets[m.tabs[0]] == nil {
		t.Fatal("no pooled widget")
	}
	m.showTabContextMenu(m.tabs[0], &fyne.PointEvent{}) // must not panic
}

// TestRequestTabSecondaryTapWithoutHandler: an unwired widget ignores the
// gesture instead of panicking.
func TestRequestTabSecondaryTapWithoutHandler(t *testing.T) {
	test.NewApp()
	rt := newRequestTab(nil, nil)
	test.TapSecondary(rt) // must not panic
}

// TestCloseTabsReclaimsDroppedResponses: the bulk close sums every dropped
// tab's cached body into one reclaim — two half-threshold bodies (each too
// small to reclaim on its own) must fire once when dropped together.
func TestCloseTabsReclaimsDroppedResponses(t *testing.T) {
	m, _, _ := newTabUI(t)
	calls, wait := captureReclaim(t)

	half := strings.Repeat("y", memTrimThreshold/2)
	openThreeTabs(t, m)
	m.deliverResponse(m.tabs[1], &tabResponse{rawBody: []byte(half), status: "200"})
	m.deliverResponse(m.tabs[2], &tabResponse{rawBody: []byte(half), status: "200"})
	if got := calls.Load(); got != 0 {
		t.Fatalf("setup fired %d reclaims, want 0 (each body is below threshold)", got)
	}

	m.closeTabs(m.tabsRightOf(m.tabs[0]), m.tabs[0])
	if !wait() {
		t.Fatal("closeTabs did not reclaim the summed cached bodies")
	}
	if len(m.tabs) != 1 || m.tabs[0].resp != nil {
		t.Errorf("survivor state: tabs=%d resp=%v", len(m.tabs), m.tabs[0].resp)
	}
}

// TestRequestCloseTabsConfirmText pins the dialog copy, including the plural
// count when more than one scratch tab would be discarded.
func TestRequestCloseTabsConfirmText(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.newScratchTab()
	m.currentRequest.URL = "https://x/one"
	m.newScratchTab()
	m.currentRequest.URL = "https://x/two"
	anchor := m.tabs[0]

	m.requestCloseTabs(m.tabsOtherThan(anchor), anchor)
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no confirm dialog on screen")
	}
	var texts []string
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Label:
			texts = append(texts, v.Text)
		case *canvas.Text:
			texts = append(texts, v.Text)
		}
	})
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"Discard tabs?", "2 unsaved requests will be lost."} {
		if !strings.Contains(joined, want) {
			t.Errorf("dialog text %q lacks %q", joined, want)
		}
	}
	test.Tap(buttonByText(top, "Yes"))
	if len(m.tabs) != 1 {
		t.Errorf("after Yes: %d tabs, want 1", len(m.tabs))
	}
}

// TestRequestCloseTabsHeadlessClosesSilently: with no window to parent a
// dialog, a scratch tab with content is closed without asking (the same
// headless rule as requestCloseTab).
func TestRequestCloseTabsHeadlessClosesSilently(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.newScratchTab()
	m.currentRequest.URL = "https://x/unsaved"
	anchor := m.tabs[0]
	m.requestCloseTabs(m.tabsOtherThan(anchor), anchor)
	if len(m.tabs) != 1 || m.tabs[0] != anchor {
		t.Errorf("tabs=%d, want just the anchor", len(m.tabs))
	}
}

// openSecondCollection writes and opens a second copy of the tab fixture so
// cross-collection cases have tabs from two collections; returns its dir.
func openSecondCollection(t *testing.T, m *MainUI) string {
	t.Helper()
	dir := writeTabTestCollection(t)
	if err := m.sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	m.Tree.Refresh()
	return dir
}

// TestSaveTabBackgroundCrossCollection: Save on a background tab from another
// collection switches the active collection to it and writes that collection,
// leaving the previously active one untouched.
func TestSaveTabBackgroundCrossCollection(t *testing.T) {
	m, sess, dir0 := newTabUI(t)
	dir1 := openSecondCollection(t, m)
	m.openOrActivate("0/r0") // First in collection 0
	m.currentRequest.URL = "https://x/edited-in-col0"
	m.openOrActivate("1/r0") // First in collection 1, active
	if sess.ActiveCollection() != 1 {
		t.Fatalf("setup: active collection = %d, want 1", sess.ActiveCollection())
	}
	m.saveTab(m.tabs[0])
	if sess.ActiveCollection() != 0 || m.activeTabIdx != 0 {
		t.Errorf("active collection/tab = %d/%d, want 0/0", sess.ActiveCollection(), m.activeTabIdx)
	}
	col0, err := storage.Load(dir0)
	if err != nil {
		t.Fatalf("Load col0: %v", err)
	}
	if col0.Requests[0].URL != "https://x/edited-in-col0" {
		t.Errorf("collection 0 on disk URL = %q, want the edit", col0.Requests[0].URL)
	}
	col1, err := storage.Load(dir1)
	if err != nil {
		t.Fatalf("Load col1: %v", err)
	}
	if col1.Requests[0].URL != "https://x/first" {
		t.Errorf("collection 1 was rewritten: URL = %q", col1.Requests[0].URL)
	}
}

// TestSaveRequestTargetsEditorCollection (regression): selecting another
// collection's row in the sidebar switches the session's active collection
// without switching tabs; a save must still write the edited request's own
// collection, not the newly active one.
func TestSaveRequestTargetsEditorCollection(t *testing.T) {
	m, sess, dir0 := newTabUI(t)
	dir1 := openSecondCollection(t, m)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited-in-col0"
	m.Tree.Select("1") // the sidebar click on collection 1's row
	if sess.ActiveCollection() != 1 {
		t.Fatalf("setup: sidebar select did not activate collection 1 (got %d)", sess.ActiveCollection())
	}

	m.tabContextMenuItems(m.tabs[0])[0].Action() // Save on the (still active) tab
	if m.Status.Text != "Saved: First" {
		t.Errorf("status = %q", m.Status.Text)
	}
	if sess.ActiveCollection() != 0 {
		t.Errorf("active collection = %d after save, want 0 (the request's)", sess.ActiveCollection())
	}
	col0, err := storage.Load(dir0)
	if err != nil {
		t.Fatalf("Load col0: %v", err)
	}
	if col0.Requests[0].URL != "https://x/edited-in-col0" {
		t.Errorf("collection 0 on disk URL = %q, want the edit", col0.Requests[0].URL)
	}
	col1, err := storage.Load(dir1)
	if err != nil {
		t.Fatalf("Load col1: %v", err)
	}
	if col1.Requests[0].URL != "https://x/first" {
		t.Errorf("collection 1 was rewritten: URL = %q", col1.Requests[0].URL)
	}
	if m.isTabDirty(m.tabs[0]) {
		t.Error("tab still dirty after save")
	}
}
