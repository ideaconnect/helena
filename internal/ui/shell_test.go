package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestNewMainUIBuilds verifies that NewMainUI produces a non-nil root, seeds
// the expected default Method and Workspace, wires the right tab counts, and
// can be laid out in a headless window without panicking.
func TestNewMainUIBuilds(t *testing.T) {
	test.NewApp()

	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)

	if m.Root() == nil {
		t.Fatal("Root() is nil")
	}
	if m.Method.Selected() != "GET" {
		t.Errorf("default method = %q, want GET", m.Method.Selected())
	}
	if m.Workspace.Selected != "Default" {
		t.Errorf("workspace = %q, want Default", m.Workspace.Selected)
	}
	if len(m.Request.Items) != 7 || len(m.Response.Items) != 2 {
		t.Errorf("tabs = %d/%d, want 7/2", len(m.Request.Items), len(m.Response.Items))
	}

	// Lay it out in a headless window to catch construction/layout panics.
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(1100, 720))
	w.Close()
}

// TestSendOrAbortDispatch verifies that sendOrAbort calls sendCancel
// when a Send is in flight (and does NOT clear it — the goroutine's
// fyne.Do path owns the reset), and that resetSendButton restores the
// Send button to its default appearance.
func TestSendOrAbortDispatch(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	// Default state: icon set, no text.
	if m.Send.Icon == nil {
		t.Error("initial Send button has no icon (expected send-diagonal-solid)")
	}
	if m.Send.Text != "" {
		t.Errorf("initial Send button text = %q, want empty (icon-only)", m.Send.Text)
	}
	if m.sendCancel != nil {
		t.Error("initial sendCancel is non-nil")
	}

	// Simulate an in-flight Send by stashing a fake cancel func.
	cancelled := false
	m.sendCancel = func() { cancelled = true }
	m.Send.SetIcon(nil)
	m.Send.SetText("Abort")

	m.sendOrAbort()
	if !cancelled {
		t.Error("sendOrAbort with active sendCancel did not call cancel()")
	}
	if m.sendCancel == nil {
		t.Error("sendOrAbort cleared sendCancel; goroutine teardown should own that")
	}
	if m.Send.Text != "Abort" {
		t.Errorf("sendOrAbort changed button text to %q; goroutine teardown owns that too", m.Send.Text)
	}

	// resetSendButton is the canonical teardown helper.
	m.resetSendButton()
	if m.sendCancel != nil {
		t.Error("resetSendButton left sendCancel non-nil")
	}
	if m.Send.Text != "" {
		t.Errorf("resetSendButton text = %q, want empty (icon-only)", m.Send.Text)
	}
	if m.Send.Icon == nil {
		t.Error("resetSendButton did not restore the icon")
	}
}
