package ui

import (
	"encoding/json"
	"fmt"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// Unsaved-edit tracking and the quit confirmation live here (#139).
//
// Request-field edits (headers, body, params, auth, scripts, chain, assertions,
// docs) land on the live tree node and persist only on an explicit Save — every
// other mutation (add / rename / delete / move, and collection / folder / env /
// global variables) saves to disk as it happens. So the only work that a quit
// can silently drop is per-request editor edits pending a Save, plus scratch
// tabs never saved into a collection. Both are anchored to an open tab, so the
// tab strip is the unit of dirty tracking.
//
// Dirtiness is a value comparison, never an edited-flag: each tab caches the
// request as it looked when last in sync with disk (openTab.cleanSnapshot) and
// a tab is unsaved when the live request marshals to something different. This
// mirrors saveRequest's baseline approach — no missed write-back path can hide
// an edit, because nothing sets a flag; the state itself is compared.

// marshalRequestState serializes r to the canonical JSON used for dirty
// comparison. model.Request has no maps, so the encoding is deterministic for a
// given value (stable field + slice order), which is what makes an equality
// check on the bytes sound.
func marshalRequestState(r *model.Request) string {
	if r == nil {
		return ""
	}
	b, err := json.Marshal(r)
	if err != nil {
		// A request that can't be marshaled can't be compared; treat it as a
		// distinct, ever-changing state so the quit guard errs toward asking.
		return "\x00unmarshalable"
	}
	return string(b)
}

// liveRequestState returns the marshaled current state of a tab's request:
// scratchReq for a scratch tab, else the live tree node resolved from the tab's
// (collection, requestID). ok is false when a tree-backed request has vanished.
func (m *MainUI) liveRequestState(t *openTab) (string, bool) {
	if t.scratch {
		return marshalRequestState(t.scratchReq), true
	}
	_, req, ok := m.sess.LocateRequest(t.collection, t.requestID)
	if !ok {
		return "", false
	}
	return marshalRequestState(req), true
}

// captureCleanSnapshot records a tree-backed tab's in-sync-with-disk baseline
// the first time it is loaded (post URL-fold). It never overwrites an existing
// snapshot: a re-activation must not rebaseline a tab whose request was edited
// then switched away from — that would hide the edit. Scratch tabs are skipped
// (they compare against the blank template via scratchHasContent). Callers pass
// the freshly loaded tab from activateTab.
func (m *MainUI) captureCleanSnapshot(t *openTab) {
	if t == nil || t.scratch || t.cleanSnapshot != "" {
		return
	}
	if s, ok := m.liveRequestState(t); ok {
		t.cleanSnapshot = s
	}
}

// refreshCleanSnapshots rebaselines every tab backed by collection dir after a
// successful save of that collection. Saving one request flushes the whole
// collection to disk (saveCollection writes the entire tree), so all its open
// tabs — not just the active one — are now in sync and must adopt the new
// baseline, or an unrelated tab in the same collection would read as still
// unsaved.
func (m *MainUI) refreshCleanSnapshots(dir string) {
	for _, t := range m.tabs {
		if t.scratch || t.collection != dir {
			continue
		}
		if s, ok := m.liveRequestState(t); ok {
			t.cleanSnapshot = s
		}
	}
}

// unsavedTabCount reports how many open tabs hold edits not yet on disk: scratch
// tabs with content, and tree-backed tabs whose live request diverges from its
// clean snapshot. It flushes the debounced body editor into the active request
// first (as saveRequest does), so an in-progress body edit is counted.
func (m *MainUI) unsavedTabCount() int {
	m.syncBodyFromEditor()
	n := 0
	for _, t := range m.tabs {
		if t.scratch {
			if scratchHasContent(t.scratchReq) {
				n++
			}
			continue
		}
		if t.cleanSnapshot == "" {
			continue // never loaded → never edited
		}
		if cur, ok := m.liveRequestState(t); ok && cur != t.cleanSnapshot {
			n++
		}
	}
	return n
}

// hasUnsavedEdits reports whether any open tab holds work a quit would drop.
func (m *MainUI) hasUnsavedEdits() bool { return m.unsavedTabCount() > 0 }

// ConfirmQuit is the single gate the window-close intercept funnels through
// (#139). With no unsaved work — or no window to parent a dialog (headless) —
// it runs onQuit immediately. Otherwise it asks the user to confirm discarding
// the unsaved edits and runs onQuit only if they accept; Cancel leaves the app
// running so they can Save. The quitting flag suppresses a second dialog when
// the close button is clicked again while the first is up.
func (m *MainUI) ConfirmQuit(onQuit func()) {
	if onQuit == nil {
		return
	}
	n := m.unsavedTabCount()
	if n == 0 || m.win == nil {
		onQuit()
		return
	}
	if m.quitting {
		return // a confirm is already on screen
	}
	m.quitting = true
	noun := "request has"
	if n > 1 {
		noun = "requests have"
	}
	msg := fmt.Sprintf(
		"%d %s unsaved changes that will be lost.\n\nCancel and press Ctrl+S to save first.",
		n, noun,
	)
	dialog.NewCustomConfirm(
		"Unsaved changes", "Discard & quit", "Cancel",
		widget.NewLabel(msg),
		func(discard bool) {
			m.quitting = false
			if discard {
				onQuit()
			}
		},
		m.win,
	).Show()
}
