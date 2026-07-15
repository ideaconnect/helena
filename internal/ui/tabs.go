package ui

import (
	"fmt"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/model"
)

// openTab is one entry in the editor tab strip — an open request editor.
//
// Identity is requestID: for tree-backed tabs it is the target's persistent
// model.Request.ID, so the tab survives request reordering / insertion /
// deletion (its node ID is re-derived from the ID by reconcileTabs and is
// never persisted as truth). Scratch tabs (the "+" affordance) are not in any
// collection: they own their request value in scratchReq and carry a synthetic
// requestID only for dedup. resp caches the tab's last response so switching
// tabs restores it.
type openTab struct {
	requestID  string
	collection string // owning collection dir; "" for scratch
	scratch    bool
	scratchReq *model.Request // owned request for scratch tabs; nil otherwise
	nodeID     string         // cached live tree node ID; re-derived by reconcileTabs
	resp       *tabResponse   // nil until a Send completes on this tab
	// cleanSnapshot is the JSON-marshaled request captured the first time the
	// tab is loaded (post URL-fold) and refreshed after each save of its
	// collection. It is the "last in sync with disk" baseline: the quit guard
	// (quit.go) flags the tab as unsaved when the live request marshals to
	// something different. Empty means never loaded — a tree-backed request
	// can't be edited before it is loaded, so an empty snapshot reads as clean.
	// Always empty for scratch tabs (they use scratchHasContent instead).
	cleanSnapshot string
}

// displayBodyCap bounds how much of a response body the shared viewer
// renders (its WithMaxInputBytes): parsing is synchronous on the UI goroutine
// at ~5-7x the source size, so an uncapped 100 MiB body (the HTTP cap's
// default) would freeze the UI for a ~600 MB parse. The full body always
// stays in the tab cache for Save response; applyResponse flags the
// truncation on the status line. Must stay above memTrimThreshold, or the
// viewer-driven reclaim sites (which measure len(m.pv.Source()), capped to
// this value) would never fire.
const displayBodyCap = 16 << 20 // 16 MiB

// tabResponse is the cached, re-renderable state of a tab's last response:
// the raw body bytes (fed to the PrettyView with FormatAuto), the formatted
// header dump, the status line, an optional CORS banner string, and the
// console output. isError routes the apply path to a plain-text error display.
//
// rawBody is handed to the PrettyView zero-copy (SetData retains the slice),
// so the tab cache and the viewer share one buffer instead of holding a full
// copy each. Nothing may mutate it after delivery.
type tabResponse struct {
	rawBody     []byte
	headersText string
	status      string
	cors        string
	console     []string
	isError     bool
	errText     string
}

// activeTab returns the currently active tab, or nil when none is active.
func (m *MainUI) activeTab() *openTab {
	if m.activeTabIdx < 0 || m.activeTabIdx >= len(m.tabs) {
		return nil
	}
	return m.tabs[m.activeTabIdx]
}

// tabIndexOf returns the slice index of t, or -1 when it is not open.
func (m *MainUI) tabIndexOf(t *openTab) int {
	for i, ot := range m.tabs {
		if ot == t {
			return i
		}
	}
	return -1
}

// tabIndexByIdentity returns the index of the open tab matching the full
// (collection, requestID) identity, or -1. Request.ID is unique only within a
// collection, so dedup must include the owning collection directory —
// otherwise two collections that happen to share a Request.ID collapse onto
// one tab and opening B activates A's tab.
func (m *MainUI) tabIndexByIdentity(collection, id string) int {
	for i, t := range m.tabs {
		if t.requestID == id && t.collection == collection {
			return i
		}
	}
	return -1
}

// openOrActivate opens a tab for the tree request at nodeID, or activates its
// existing tab when one is already open. No-op for non-request nodes.
func (m *MainUI) openOrActivate(nodeID string) {
	req, ok := m.sess.Tree().Request(nodeID)
	if !ok {
		return
	}
	ci := m.sess.Tree().CollectionIndex(nodeID)
	collection := m.sess.CollectionDir(ci)
	if i := m.tabIndexByIdentity(collection, req.ID); i >= 0 {
		m.activateTab(i)
		return
	}
	m.tabs = append(m.tabs, &openTab{
		requestID:  req.ID,
		collection: collection,
		nodeID:     nodeID,
	})
	m.activateTab(len(m.tabs) - 1)
}

// newScratchTab opens a blank, unsaved request in a new tab (the "+"
// affordance). The tab owns its request value until "Save As" writes it into
// a collection.
func (m *MainUI) newScratchTab() {
	m.tabs = append(m.tabs, &openTab{
		requestID:  model.NewID(),
		scratch:    true,
		scratchReq: &model.Request{Method: model.GET, Body: model.Body{Type: model.BodyNone}},
	})
	m.activateTab(len(m.tabs) - 1)
}

// openScratchWith opens req in a new unsaved (scratch) tab — used by the
// "Paste cURL" import so the parsed request lands in the editor ready to be
// reviewed and saved into a collection.
func (m *MainUI) openScratchWith(req model.Request) {
	r := req
	m.tabs = append(m.tabs, &openTab{
		requestID:  model.NewID(),
		scratch:    true,
		scratchReq: &r,
	})
	m.activateTab(len(m.tabs) - 1)
}

// activateTab makes the tab at index i active: re-derives its live node ID and
// request pointer (closing it if the request has vanished), binds the editor
// via loadRequest, restores the tab's cached response, and re-renders the
// strip. Persists the tab set. For tree-backed tabs it also makes the owning
// collection active so the environment selector and resolver follow the tab.
func (m *MainUI) activateTab(i int) {
	if i < 0 || i >= len(m.tabs) {
		return
	}
	tab := m.tabs[i]
	m.activeTabIdx = i

	if tab.scratch {
		m.currentRequest = tab.scratchReq
		m.currentRequestID = ""
		m.loadRequest(tab.scratchReq, "")
		m.Status.SetText("New request")
	} else {
		nodeID, req, ok := m.sess.LocateRequest(tab.collection, tab.requestID)
		if !ok {
			m.closeTab(tab) // request vanished out from under us
			return
		}
		tab.nodeID = nodeID
		if ci := m.sess.Tree().CollectionIndex(nodeID); ci >= 0 {
			m.sess.SetActiveCollection(ci)
			m.refreshEnvironments()
		}
		m.loadRequest(req, nodeID)
		m.Status.SetText("Loaded: " + req.Name)
	}
	// Baseline the tab against disk the first time it loads (post URL-fold), so
	// the quit guard can tell later edits from a pristine open (#139).
	m.captureCleanSnapshot(tab)
	m.applyResponse(tab.resp) // overrides the status line when a response is cached
	m.rebuildTabBar()
	m.persistTabs()
}

// requestCloseTab is the close-button entry point. A scratch tab with content
// would lose unsaved work, so it confirms first; tree-backed tabs close
// immediately — their edits live in the tree, still reachable from the sidebar,
// and the quit guard (quit.go / ConfirmQuit) is what catches them at app exit.
func (m *MainUI) requestCloseTab(t *openTab) {
	if t.scratch && scratchHasContent(t.scratchReq) && m.win != nil {
		dialog.ShowConfirm("Discard tab?", "This unsaved request will be lost.", func(yes bool) {
			if yes {
				m.closeTab(t)
			}
		}, m.win)
		return
	}
	m.closeTab(t)
}

// scratchHasContent reports whether a scratch request carries anything worth
// confirming before discarding.
func scratchHasContent(r *model.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Name) != "" ||
		strings.TrimSpace(r.URL) != "" ||
		strings.TrimSpace(r.Body.Content) != "" ||
		len(r.Params) > 0 || len(r.Headers) > 0
}

// closeTab removes t from the strip and re-points the active tab. Closing the
// active tab activates a neighbor; closing the last tab clears the editor.
func (m *MainUI) closeTab(t *openTab) {
	idx := m.tabIndexOf(t)
	if idx < 0 {
		return
	}
	// The closed tab's cached response is dropped with the tab; reclaim a large
	// one once the strip has been rebuilt (harmlessly overlaps the reclaim
	// applyResponse/clearResponsePanel fire on the active-tab paths).
	defer reclaimAfterLargeBody(cachedBodyLen(t))
	wasActive := idx == m.activeTabIdx
	// slices.Delete zeroes the vacated tail slot; a plain append-splice would
	// leave the closed tab reachable through the backing array, pinning its
	// cached response so the reclaim above could never actually free it.
	m.tabs = slices.Delete(m.tabs, idx, idx+1)

	if len(m.tabs) == 0 {
		m.activeTabIdx = -1
		m.currentRequest = nil
		m.loadRequest(nil, "")
		m.clearResponsePanel()
		m.rebuildTabBar()
		m.persistTabs()
		return
	}
	switch {
	case wasActive:
		next := idx
		if next >= len(m.tabs) {
			next = len(m.tabs) - 1
		}
		m.activateTab(next) // rebuilds the strip + persists
	case m.activeTabIdx > idx:
		m.activeTabIdx-- // active tab shifted left by the removal
		m.rebuildTabBar()
		m.persistTabs()
	default:
		m.rebuildTabBar()
		m.persistTabs()
	}
}

// closeAllTabs clears every tab and the editor without persisting. Used on
// workspace switch, where the collections reload and every cached node ID /
// pointer is invalidated. The persisted tab set is intentionally left alone
// (see WORKFLOW.md — v1 tabs are global, no per-workspace memory).
func (m *MainUI) closeAllTabs() {
	var freed int
	for _, t := range m.tabs {
		freed += cachedBodyLen(t)
	}
	defer reclaimAfterLargeBody(freed) // every tab's cached response is dropped
	m.tabs = nil
	m.activeTabIdx = -1
	m.currentRequest = nil
	m.loadRequest(nil, "")
	m.clearResponsePanel()
	if m.tabBar != nil {
		m.rebuildTabBar()
	}
}

// reconcileTabs re-derives every tree-backed tab's live node ID + pointer from
// its requestID after a tree mutation, drops tabs whose request has vanished
// (deleted, or its collection removed), and refreshes the strip. The active
// tab's editor binding is re-pointed in place so in-flight edits survive a
// sibling insert/delete that relocated the request in its slice.
func (m *MainUI) reconcileTabs() {
	if len(m.tabs) == 0 {
		return
	}
	active := m.activeTab()
	// Dropped tabs discard their cached responses; reclaim the total on every
	// exit path (closure: freed accumulates below, and one branch returns early).
	var freed int
	defer func() { reclaimAfterLargeBody(freed) }()
	kept := m.tabs[:0]
	for _, t := range m.tabs {
		if t.scratch {
			kept = append(kept, t)
			continue
		}
		nodeID, req, ok := m.sess.LocateRequest(t.collection, t.requestID)
		if !ok {
			freed += cachedBodyLen(t)
			continue // request deleted or collection removed → drop the tab
		}
		t.nodeID = nodeID
		kept = append(kept, t)
		if t == active {
			m.currentRequest = req
			m.currentRequestID = nodeID
		}
	}
	// Zero the filtered-out tail slots: kept aliases m.tabs' backing array, so
	// without this the dropped tabs (and their cached responses) stay reachable
	// and the deferred reclaim above could never actually free them.
	clear(m.tabs[len(kept):])
	m.tabs = kept

	m.activeTabIdx = m.tabIndexOf(active)
	if m.activeTabIdx < 0 {
		if len(m.tabs) > 0 {
			m.activateTab(0) // active tab was dropped — fall to the first survivor
			return
		}
		m.activeTabIdx = -1
		m.currentRequest = nil
		m.loadRequest(nil, "")
		m.clearResponsePanel()
	}
	m.rebuildTabBar()
	m.persistTabs()
}

// rebuildTabBar syncs the strip to m.tabs: one requestTab per tab (label
// resolved fresh so renames show) plus the trailing "+" button, with the active
// tab highlighted. Widgets are pooled per *openTab and reused across rebuilds —
// reordering only re-arranges the container's objects, never recreates them, so
// an in-flight drag gesture (bound to a widget instance) survives a live
// reorder. The overflow button is shown only when at least one tab is open.
func (m *MainUI) rebuildTabBar() {
	if m.tabBar == nil {
		return
	}
	if m.tabWidgets == nil {
		m.tabWidgets = map[*openTab]*requestTab{}
	}
	present := make(map[*openTab]bool, len(m.tabs))
	for _, t := range m.tabs {
		present[t] = true
	}
	for t := range m.tabWidgets {
		if !present[t] {
			delete(m.tabWidgets, t)
		}
	}

	// Flush the debounced body editor once so the active tab's in-progress body
	// edit is reflected in its dirty marker (isTabDirty compares marshaled state).
	m.syncBodyFromEditor()
	objs := make([]fyne.CanvasObject, 0, len(m.tabs)+1)
	for i, t := range m.tabs {
		rt := m.tabWidgets[t]
		if rt == nil {
			t := t
			rt = newRequestTab(
				func() { m.activateTab(m.tabIndexOf(t)) },
				func() { m.requestCloseTab(t) },
			)
			rt.onDrag = func(e *fyne.DragEvent) { m.dragTab(t, e) }
			rt.onDragEnd = m.dragEnd
			m.tabWidgets[t] = rt
		}
		method, name := m.tabLabel(t)
		rt.setTab(method, name, m.isTabDirty(t))
		rt.setActive(i == m.activeTabIdx)
		objs = append(objs, rt)
	}
	objs = append(objs, m.newTabBtn)
	m.tabBar.Objects = objs
	m.tabBar.Refresh()

	if m.tabOverflowBtn != nil {
		if len(m.tabs) > 0 {
			m.tabOverflowBtn.Show()
		} else {
			m.tabOverflowBtn.Hide()
		}
	}
}

// dragTab live-reorders the strip while tab t is dragged: it re-inserts t at the
// slot implied by the pointer's X among the other tabs' on-screen centers. Reuses
// pooled widgets so the gesture stays bound through the rebuild.
func (m *MainUI) dragTab(t *openTab, e *fyne.DragEvent) {
	if m.tabIndexOf(t) < 0 {
		return
	}
	centers, ok := m.otherTabCenters(t)
	if !ok {
		return
	}
	m.applyDragTarget(t, dropIndex(centers, e.AbsolutePosition.X))
}

// otherTabCenters returns the on-screen horizontal centers of every tab except
// t, in current order. Returns ok=false when geometry is unavailable (no driver
// or an unrealized widget) so the caller leaves the order untouched.
func (m *MainUI) otherTabCenters(t *openTab) ([]float32, bool) {
	drv := fyne.CurrentApp().Driver()
	if drv == nil {
		return nil, false
	}
	centers := make([]float32, 0, len(m.tabs))
	for _, ot := range m.tabs {
		if ot == t {
			continue
		}
		rt := m.tabWidgets[ot]
		if rt == nil {
			return nil, false
		}
		pos := drv.AbsolutePositionForObject(rt)
		centers = append(centers, pos.X+rt.Size().Width/2)
	}
	return centers, true
}

// applyDragTarget moves t to insertion index target, keeping the active tab
// active, and re-syncs the strip. No-op when the order is unchanged.
func (m *MainUI) applyDragTarget(t *openTab, target int) {
	newTabs := moveTabModel(m.tabs, t, target)
	if slices.Equal(newTabs, m.tabs) {
		return
	}
	active := m.activeTab()
	m.tabs = newTabs
	m.activeTabIdx = m.tabIndexOf(active)
	m.rebuildTabBar()
}

// dragEnd persists the new tab order once a drag completes; the live reorder
// already updated the strip.
func (m *MainUI) dragEnd() { m.persistTabs() }

// moveTabModel returns tabs with dragged repositioned to insertion index target
// among the other tabs (target clamped into range).
func moveTabModel(tabs []*openTab, dragged *openTab, target int) []*openTab {
	others := make([]*openTab, 0, len(tabs))
	for _, t := range tabs {
		if t != dragged {
			others = append(others, t)
		}
	}
	if target < 0 {
		target = 0
	}
	if target > len(others) {
		target = len(others)
	}
	out := make([]*openTab, 0, len(tabs))
	out = append(out, others[:target]...)
	out = append(out, dragged)
	out = append(out, others[target:]...)
	return out
}

// dropIndex returns how many of the given tab centers lie left of pointerX —
// the insertion index for a tab dragged to pointerX.
func dropIndex(centers []float32, pointerX float32) int {
	n := 0
	for _, c := range centers {
		if pointerX > c {
			n++
		}
	}
	return n
}

// tabMenuItems builds the overflow menu: one entry per open tab (method +
// name), the active one checked, each activating its tab on selection.
func (m *MainUI) tabMenuItems() []*fyne.MenuItem {
	items := make([]*fyne.MenuItem, 0, len(m.tabs))
	for i, t := range m.tabs {
		i := i
		method, name := m.tabLabel(t)
		if name == "" {
			name = "Untitled"
		}
		label := name
		if method != "" {
			label = method + "  " + name
		}
		if m.isTabDirty(t) {
			label += " *"
		}
		mi := fyne.NewMenuItem(label, func() { m.activateTab(i) })
		mi.Checked = i == m.activeTabIdx
		items = append(items, mi)
	}
	return items
}

// showTabMenu pops up the overflow menu listing every open tab, for jumping to
// one that has scrolled out of view in a crowded strip.
func (m *MainUI) showTabMenu() {
	if len(m.tabs) == 0 || m.tabOverflowBtn == nil {
		return
	}
	canv := fyne.CurrentApp().Driver().CanvasForObject(m.tabOverflowBtn)
	if canv == nil {
		return
	}
	pop := widget.NewPopUpMenu(fyne.NewMenu("", m.tabMenuItems()...), canv)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(m.tabOverflowBtn)
	pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+m.tabOverflowBtn.Size().Height))
}

// refreshActiveTabLabel re-renders the strip so the active tab's method chip /
// name reflect an in-editor change (e.g. picking a new method). Cheap; skipped
// while loadRequest is pushing values in (m.loading) since activateTab rebuilds
// at the end of a load anyway.
func (m *MainUI) refreshActiveTabLabel() {
	if len(m.tabs) > 0 {
		m.rebuildTabBar()
	}
}

// refreshActiveTabDirty re-evaluates just the active tab's dirty marker and
// updates its widget in place. It is the per-edit hook the request editors call
// on every change (URL, body, headers, params, auth, …), so the asterisk
// appears the moment content diverges from disk and clears when it matches
// again (e.g. an undo back to the saved state). Only the active tab can change
// dirtiness from an in-editor edit, so this avoids rebuilding the whole strip
// per keystroke; a tab switch / save rebuilds every marker via rebuildTabBar.
func (m *MainUI) refreshActiveTabDirty() {
	if m.loading {
		return
	}
	t := m.activeTab()
	if t == nil {
		return
	}
	rt := m.tabWidgets[t]
	if rt == nil {
		return
	}
	m.syncBodyFromEditor() // count an in-progress (debounced) body edit
	method, name := m.tabLabel(t)
	rt.setTab(method, name, m.isTabDirty(t))
}

// tabLabel returns the method + name to show on a tab, resolved from live
// state (so renames and scratch edits surface on the next rebuild).
func (m *MainUI) tabLabel(t *openTab) (method, name string) {
	if t.scratch {
		return string(t.scratchReq.Method), t.scratchReq.Name
	}
	if _, req, ok := m.sess.LocateRequest(t.collection, t.requestID); ok {
		return string(req.Method), req.Name
	}
	return "", ""
}

// persistTabs writes the tree-backed tabs (scratch tabs are not persistable)
// and the active index to the session in one call.
func (m *MainUI) persistTabs() {
	var out []config.UIOpenTab
	active := 0
	at := m.activeTab()
	for _, t := range m.tabs {
		if t.scratch {
			continue
		}
		if t == at {
			active = len(out)
		}
		out = append(out, config.UIOpenTab{Collection: t.collection, RequestID: t.requestID})
	}
	m.sess.SetOpenTabs(out, active)
}

// restoreTabs reopens the persisted tabs on launch, activating whichever was
// active. Tabs whose request no longer resolves are skipped. Falls back to the
// legacy single OpenRequest when no tab set is persisted (first run after the
// upgrade).
func (m *MainUI) restoreTabs() {
	tabs, active := m.sess.OpenTabs()
	if len(tabs) == 0 {
		if id := m.sess.OpenRequest(); id != "" {
			m.openOrActivate(id)
		}
		return
	}
	activeReqID := ""
	if active >= 0 && active < len(tabs) {
		activeReqID = tabs[active].RequestID
	}
	for _, ut := range tabs {
		nodeID, req, ok := m.sess.LocateRequest(ut.Collection, ut.RequestID)
		if !ok {
			continue
		}
		m.tabs = append(m.tabs, &openTab{
			requestID:  req.ID,
			collection: ut.Collection,
			nodeID:     nodeID,
		})
	}
	if len(m.tabs) == 0 {
		return
	}
	idx := 0
	for i, t := range m.tabs {
		if t.requestID == activeReqID {
			idx = i
			break
		}
	}
	m.activateTab(idx)
}

// deliverResponse attaches a freshly captured response to the tab that
// initiated the Send and repaints the shared panel only if that tab is still
// active (the user may have switched tabs mid-Send). A nil initTab means the
// Send had no open tab (bare URL) — then the panel is always repainted. Runs
// on the UI goroutine (inside the Send's fyne.Do).
// cachedBodyLen is the size of a tab's cached response body (nil-safe), fed to
// reclaimAfterLargeBody when the cache is dropped or replaced.
func cachedBodyLen(t *openTab) int {
	if t == nil || t.resp == nil {
		return 0
	}
	return len(t.resp.rawBody)
}

func (m *MainUI) deliverResponse(initTab *openTab, resp *tabResponse) {
	old := cachedBodyLen(initTab)
	if initTab != nil {
		initTab.resp = resp
	}
	if initTab == nil || m.activeTab() == initTab {
		m.applyResponse(resp) // reclaims the displayed (same) outgoing body itself
	} else {
		// The tab isn't displayed, so applyResponse never runs: reclaim the
		// replaced cached body here or a large one lingers in RSS.
		reclaimAfterLargeBody(old)
	}
}

// applyResponse pushes a cached response into the shared response panel, or
// clears it when r is nil. Used both by the Send completion (for the active
// tab) and by activateTab (restoring a tab's cached response on switch).
func (m *MainUI) applyResponse(r *tabResponse) {
	if r == nil {
		m.clearResponsePanel()
		return
	}
	// The outgoing body's parsed model becomes garbage once we SetData below;
	// reclaim it off-UI if it was large so RSS doesn't ratchet across a session.
	freed := len(m.pv.Source())
	defer reclaimAfterLargeBody(freed)
	m.setScriptConsole(r.console)
	m.Response.SelectIndex(0) // Body
	if r.isError {
		// Force raw so an error string that resembles JSON/XML isn't rendered
		// as structured data.
		m.pv.SetData([]byte(r.errText), prettyview.FormatRaw)
		m.headersText.SetText("")
		m.Status.SetText(r.status)
		m.corsBanner.Hide()
		// Non-transient surface so the failure isn't lost when the status line
		// is overwritten or the user switches response sub-tabs (#51).
		m.showErrorBanner(r.status)
		return
	}
	m.pv.SetData(r.rawBody, prettyview.FormatAuto)
	m.headersText.SetText(r.headersText)
	status := r.status
	if len(r.rawBody) > displayBodyCap {
		// The viewer rendered only the first displayBodyCap bytes (its
		// WithMaxInputBytes); the tab cache — and so Save response — keeps all.
		status += fmt.Sprintf(" · display truncated to %d MiB — Save response for the full body", displayBodyCap>>20)
	}
	m.Status.SetText(status)
	m.hideErrorBanner()
	if r.cors != "" {
		m.corsBanner.Text = r.cors
		m.corsBanner.Refresh()
		m.corsBanner.Show()
	} else {
		m.corsBanner.Hide()
	}
}

// clearResponsePanel blanks every response widget.
func (m *MainUI) clearResponsePanel() {
	defer reclaimAfterLargeBody(len(m.pv.Source()))
	m.pv.SetData(nil, prettyview.FormatRaw)
	m.headersText.SetText("")
	m.corsBanner.Hide()
	m.hideErrorBanner()
	m.setScriptConsole(nil)
}

// saveScratchTabAs prompts for a name + destination container and writes the
// scratch tab's request into that collection, converting the tab to
// tree-backed. The destination list spans every loaded collection's containers
// (collection roots + folders).
func (m *MainUI) saveScratchTabAs(t *openTab) {
	if m.win == nil {
		return
	}
	containers := m.sess.ContainerPaths()
	if len(containers) == 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	nameEntry := widget.NewEntry()
	nameEntry.SetText(t.scratchReq.Name)
	nameEntry.SetPlaceHolder("Request name")
	labels := make([]string, len(containers))
	for i, c := range containers {
		labels[i] = c.Label
	}
	dest := widget.NewSelect(labels, nil)
	dest.SetSelectedIndex(0)
	form := widget.NewForm(
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Save to", dest),
	)
	d := dialog.NewCustomConfirm("Save As", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			m.Status.SetText("Name cannot be empty")
			return
		}
		di := dest.SelectedIndex()
		if di < 0 || di >= len(containers) {
			return
		}
		m.commitScratchTab(t, containers[di].NodeID, name)
	}, m.win)
	d.Resize(fyne.NewSize(440, 200))
	d.Show()
}

// commitScratchTab inserts the scratch tab's request under parentNodeID, makes
// that collection active (so the save targets the right file), then converts
// the tab to a tree-backed tab bound to the new on-disk request.
func (m *MainUI) commitScratchTab(t *openTab, parentNodeID, name string) {
	ci := m.sess.Tree().CollectionIndex(parentNodeID)
	if ci < 0 {
		m.Status.SetText("Invalid destination")
		return
	}
	req := *t.scratchReq
	req.Name = name
	// AddRequestValue resolves + persists by the parent's collection index, so it
	// does not require the active collection to be switched first. Defer the
	// active-collection/env switch until AFTER the save succeeds — otherwise a
	// failed save would leave the session pointing at a collection whose data
	// wasn't persisted, diverging UI state from disk.
	newNodeID, err := m.sess.AddRequestValue(parentNodeID, req)
	if err != nil {
		if m.win != nil {
			dialog.ShowError(err, m.win)
		}
		m.Status.SetText("Save failed: " + err.Error())
		return
	}
	newReq, ok := m.sess.Tree().Request(newNodeID)
	if !ok {
		m.Status.SetText("Saved, but could not reopen the request")
		return
	}
	m.sess.SetActiveCollection(ci)
	t.scratch = false
	t.scratchReq = nil
	t.collection = m.sess.CollectionDir(ci)
	t.nodeID = newNodeID
	t.requestID = newReq.ID

	m.Tree.Refresh()
	m.Tree.OpenBranch(parentNodeID)
	m.activateTab(m.tabIndexOf(t)) // rebind editor to the on-disk request + persist
	m.Status.SetText("Saved: " + name)
}
