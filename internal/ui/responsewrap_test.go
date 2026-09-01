package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/session"
)

// newWrapUI builds a headless MainUI backed by a real config file at cfgPath, so
// a test can reopen the session and assert what actually reached disk.
func newWrapUI(t *testing.T, cfgPath string) (*MainUI, *session.Session) {
	t.Helper()
	test.NewApp()
	sess, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return NewMainUI(sess), sess
}

// findWrapToggle returns the response viewer toolbar's soft-wrap button — the
// control a user actually clicks — located by its Font Awesome icon resource.
func findWrapToggle(t *testing.T, root fyne.CanvasObject) fyne.Tappable {
	t.Helper()
	var found fyne.Tappable
	walkObjects(root, func(o fyne.CanvasObject) {
		b, ok := o.(*ttwidget.Button)
		if !ok || b.Icon == nil || b.Icon.Name() != "fa-wrap-text.svg" {
			return
		}
		if found != nil {
			t.Fatal("more than one wrap toggle in the shell; the test would tap an arbitrary one")
		}
		found = b
	})
	if found == nil {
		t.Fatal("no wrap toggle found in the shell (response toolbar not built?)")
	}
	return found
}

// TestResponseWrapDefaultsOff verifies a fresh profile opens the response viewer
// in horizontal-scroll mode (the pre-existing default), not wrapped.
func TestResponseWrapDefaultsOff(t *testing.T) {
	m, _ := newWrapUI(t, filepath.Join(t.TempDir(), "config.yml"))
	if got := m.pv.Wrap(); got != prettyview.WrapNone {
		t.Errorf("fresh profile wrap = %v, want WrapNone", got)
	}
}

// TestResponseWrapRestoredFromSession verifies the viewer opens wrapped when the
// session says the user left it on — the "remember the state" half of the feature.
func TestResponseWrapRestoredFromSession(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	test.NewApp()
	sess, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	sess.SetResponseWrap(true)

	// A fresh session over the same config is what the next launch really does.
	restored, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New (reopen): %v", err)
	}
	m := NewMainUI(restored)
	if got := m.pv.Wrap(); got != prettyview.WrapWord {
		t.Errorf("restored wrap = %v, want WrapWord", got)
	}
	// The toolbar button must *look* on too, or a restored-but-unhighlighted
	// toggle reads as "wrap is off" and the next tap turns it off instead of on.
	btn, ok := findWrapToggle(t, m.Root()).(*ttwidget.Button)
	if !ok {
		t.Fatal("wrap toggle is not a *ttwidget.Button")
	}
	if btn.Importance != widget.HighImportance {
		t.Errorf("restored wrap toggle importance = %v, want HighImportance (highlighted)", btn.Importance)
	}
}

// TestResponseWrapToggleWritesBack verifies flipping the viewer's wrap mode
// persists it, so the *next* launch restores what this one ended with.
func TestResponseWrapToggleWritesBack(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	m, sess := newWrapUI(t, cfgPath)

	m.pv.SetWrap(prettyview.WrapWord)
	if !sess.ResponseWrap() {
		t.Fatal("SetWrap(WrapWord) did not reach the session")
	}
	reopened, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New (reopen): %v", err)
	}
	if !reopened.ResponseWrap() {
		t.Error("wrap-on did not reach the config file")
	}

	m.pv.SetWrap(prettyview.WrapNone)
	if sess.ResponseWrap() {
		t.Fatal("SetWrap(WrapNone) did not reach the session")
	}
	reopened2, err := session.New(cfgPath)
	if err != nil {
		t.Fatalf("session.New (reopen 2): %v", err)
	}
	if reopened2.ResponseWrap() {
		t.Error("wrap-off did not reach the config file")
	}
}

// TestResponseWrapToolbarToggleWritesBack verifies the write-back is wired to the
// control the user actually clicks — the viewer's own toolbar wrap button — not
// just to a direct SetWrap call.
func TestResponseWrapToolbarToggleWritesBack(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	m, sess := newWrapUI(t, cfgPath)
	win := test.NewWindow(m.Root())
	defer win.Close()
	win.Resize(fyne.NewSize(1100, 720))

	btn := findWrapToggle(t, m.Root())
	test.Tap(btn)
	if m.pv.Wrap() != prettyview.WrapWord || !sess.ResponseWrap() {
		t.Fatalf("toolbar tap: wrap=%v persisted=%v, want WrapWord/true", m.pv.Wrap(), sess.ResponseWrap())
	}
	test.Tap(btn)
	if m.pv.Wrap() != prettyview.WrapNone || sess.ResponseWrap() {
		t.Fatalf("second tap: wrap=%v persisted=%v, want WrapNone/false", m.pv.Wrap(), sess.ResponseWrap())
	}
}

// TestResponseWrapSurvivesResponseLoad verifies loading a new response body does
// not silently reset the mode — the toggle is sticky within a session too.
func TestResponseWrapSurvivesResponseLoad(t *testing.T) {
	m, sess := newWrapUI(t, filepath.Join(t.TempDir(), "config.yml"))
	m.pv.SetWrap(prettyview.WrapWord)

	m.applyResponse(&tabResponse{rawBody: []byte(`{"a":1}`), status: "200 OK"})
	if m.pv.Wrap() != prettyview.WrapWord {
		t.Errorf("wrap reset to %v by a response load", m.pv.Wrap())
	}
	if !sess.ResponseWrap() {
		t.Error("persisted wrap flipped off during a response load")
	}
}
