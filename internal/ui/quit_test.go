package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestUnsavedCleanAfterOpen: opening a request and touching nothing leaves no
// unsaved work — a pristine load must never trip the quit guard.
func TestUnsavedCleanAfterOpen(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	if m.hasUnsavedEdits() {
		t.Fatalf("pristine open reads as unsaved")
	}
	if n := m.unsavedTabCount(); n != 0 {
		t.Fatalf("unsavedTabCount = %d, want 0", n)
	}
}

// TestUnsavedAfterFieldEdit: an edit landing on the live request (as a
// write-back callback would) marks the tab unsaved.
func TestUnsavedAfterFieldEdit(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/first/edited"
	if !m.hasUnsavedEdits() {
		t.Fatalf("field edit did not mark the tab unsaved")
	}
}

// TestUnsavedBodyEditCountedAfterSync: a debounced body edit lives only in the
// editor until synced; hasUnsavedEdits must flush it first, or it goes unseen.
func TestUnsavedBodyEditCountedAfterSync(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	// Write to the editor widget only (no manual sync) — the guard is
	// responsible for pulling it into the request.
	m.BodyContent.SetData([]byte("changed body"), prettyview.FormatRaw)
	if !m.hasUnsavedEdits() {
		t.Fatalf("unsynced body edit not counted (syncBodyFromEditor not called?)")
	}
}

// TestSaveClearsUnsaved: saving the edited request rebaselines it, so a
// subsequent quit sees clean state.
func TestSaveClearsUnsaved(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/first/edited"
	m.saveRequest()
	if m.hasUnsavedEdits() {
		t.Fatalf("still unsaved after Save")
	}
}

// TestUnsavedTracksInactiveTab: edits to a request the user switched away from
// stay pending in the tree and must still be counted at quit.
func TestUnsavedTracksInactiveTab(t *testing.T) {
	m, sess, _ := newTabUI(t)
	m.openOrActivate("0/r0") // tab 0, snapshot captured
	m.openOrActivate("0/r1") // tab 1 active

	// Edit r0's node while it is inactive (its tab keeps the pre-edit snapshot).
	_, r0, ok := sess.LocateRequest(m.tabs[0].collection, m.tabs[0].requestID)
	if !ok {
		t.Fatal("locate r0")
	}
	r0.URL = "https://x/first/edited"

	if n := m.unsavedTabCount(); n != 1 {
		t.Fatalf("unsavedTabCount = %d, want 1 (inactive edited tab)", n)
	}
}

// TestReactivateDoesNotHideEdit: switching back to an edited tab must not
// rebaseline it — the snapshot is captured once and only refreshed on save.
func TestReactivateDoesNotHideEdit(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/first/edited"
	m.openOrActivate("0/r1") // switch away (edit stays in the tree)
	m.openOrActivate("0/r0") // switch back — must NOT re-snapshot the edited state
	if !m.hasUnsavedEdits() {
		t.Fatalf("re-activation hid the pending edit")
	}
}

// TestSaveRebaselinesSiblingTab: saving one request flushes the whole
// collection, so every open tab in it — not just the saved one — is now clean.
func TestSaveRebaselinesSiblingTab(t *testing.T) {
	m, sess, _ := newTabUI(t)
	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 active

	// Dirty the inactive r0 directly in the tree, then save via the active tab.
	_, r0, ok := sess.LocateRequest(m.tabs[0].collection, m.tabs[0].requestID)
	if !ok {
		t.Fatal("locate r0")
	}
	r0.URL = "https://x/first/edited"
	m.saveRequest() // saves the active collection (persists r0's edit too)

	if m.hasUnsavedEdits() {
		t.Fatalf("sibling tab not rebaselined after collection save")
	}
}

// TestScratchTabUnsaved: an empty scratch tab is not worth confirming, but one
// with content is.
func TestScratchTabUnsaved(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.newScratchTab()
	if m.hasUnsavedEdits() {
		t.Fatalf("blank scratch tab reads as unsaved")
	}
	m.currentRequest.URL = "https://x/scratch"
	if !m.hasUnsavedEdits() {
		t.Fatalf("scratch tab with content not counted")
	}
}

// writeFoldTestCollection writes a collection whose sole request stores an
// inline query in its URL — the case loadRequest folds into Params, mutating
// the live node. The quit guard must not read that fold as an edit.
func writeFoldTestCollection(t testing.TB) string {
	t.Helper()
	c := model.Collection{
		Name: "Fold API",
		Requests: []model.Request{
			{ID: "id-fold", Name: "Search", Method: model.GET, URL: "https://x/search?q=1&page=2"},
		},
	}
	dir := filepath.Join(t.TempDir(), "fold-api")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

// TestFoldedURLNotFalsePositive: opening a request with an inline-query URL
// folds it into Params (a display convenience that mutates the node). Because
// the snapshot is captured post-fold, an untouched open stays clean.
func TestFoldedURLNotFalsePositive(t *testing.T) {
	test.NewApp()
	dir := writeFoldTestCollection(t)
	sess, err := session.New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	m.openOrActivate("0/r0")
	// Sanity: the fold actually happened (URL split, query moved to Params).
	if got := m.currentRequest.URL; got != "https://x/search" {
		t.Fatalf("fold precondition: URL = %q, want folded base", got)
	}
	if m.hasUnsavedEdits() {
		t.Fatalf("post-fold pristine open reads as unsaved (false positive)")
	}
}

// TestConfirmQuitNoEditsQuitsImmediately: with nothing pending, ConfirmQuit runs
// the quit callback without a dialog.
func TestConfirmQuitNoEditsQuitsImmediately(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")

	quit := false
	m.ConfirmQuit(func() { quit = true })
	if !quit {
		t.Fatalf("ConfirmQuit did not quit with no unsaved edits")
	}
}

// TestConfirmQuitHeadlessBypasses: with unsaved edits but no window to parent a
// dialog, ConfirmQuit cannot ask, so it must not block the quit.
func TestConfirmQuitHeadlessBypasses(t *testing.T) {
	m, _, _ := newTabUI(t)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited" // dirty, but m.win is nil
	quit := false
	m.ConfirmQuit(func() { quit = true })
	if !quit {
		t.Fatalf("headless ConfirmQuit blocked the quit")
	}
}

// TestConfirmQuitCancelKeepsRunning: with unsaved edits and a window, ConfirmQuit
// shows a dialog and does not quit until the user chooses; Cancel keeps the app
// running and clears the re-entrancy guard.
func TestConfirmQuitCancelKeepsRunning(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited"

	quit := false
	m.ConfirmQuit(func() { quit = true })
	if quit {
		t.Fatalf("ConfirmQuit quit before the user confirmed")
	}
	if !m.quitting {
		t.Fatalf("quitting flag not set while dialog is up")
	}
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no confirm dialog on screen")
	}
	cancel := buttonByText(top, "Cancel")
	if cancel == nil {
		t.Fatal("Cancel button not found")
	}
	test.Tap(cancel)
	if quit {
		t.Fatalf("Cancel quit the app")
	}
	if m.quitting {
		t.Fatalf("quitting flag not cleared after Cancel")
	}
}

// TestConfirmQuitDiscardQuits: choosing "Discard & quit" runs the quit callback.
func TestConfirmQuitDiscardQuits(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited"

	quit := false
	m.ConfirmQuit(func() { quit = true })
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("no confirm dialog on screen")
	}
	discard := buttonByText(top, "Discard & quit")
	if discard == nil {
		t.Fatal("Discard button not found")
	}
	test.Tap(discard)
	if !quit {
		t.Fatalf("Discard & quit did not quit")
	}
}

// TestConfirmQuitSuppressesSecondDialog: clicking close again while the confirm
// is up must not stack a second dialog.
func TestConfirmQuitSuppressesSecondDialog(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	m.currentRequest.URL = "https://x/edited"

	m.ConfirmQuit(func() {})
	before := len(w.Canvas().Overlays().List())
	m.ConfirmQuit(func() {}) // second close click while dialog is up
	if after := len(w.Canvas().Overlays().List()); after != before {
		t.Fatalf("second ConfirmQuit stacked a dialog: %d → %d", before, after)
	}
}
