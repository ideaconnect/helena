package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestErrorBannerSurfacesFailureNonTransiently verifies that an error response
// raises the persistent error banner (visible regardless of the status line),
// a successful response clears it, and dismissing hides it (#51).
func TestErrorBannerSurfacesFailureNonTransiently(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	// Initially hidden.
	if m.errorBanner.Visible() {
		t.Fatal("error banner visible before any response")
	}

	// An error response raises and populates the banner.
	m.applyResponse(&tabResponse{isError: true, errText: "dial tcp: refused", status: "Error: dial tcp: refused"})
	if !m.errorBanner.Visible() {
		t.Error("error banner not shown for a failed response")
	}
	if !strings.Contains(m.errorBannerLabel.Text, "dial tcp") {
		t.Errorf("banner text = %q, want it to carry the error", m.errorBannerLabel.Text)
	}

	// A transient status update must NOT clear the banner (the whole point).
	m.Status.SetText("Ready")
	if !m.errorBanner.Visible() {
		t.Error("error banner cleared by a status update (should be non-transient)")
	}

	// A subsequent successful response clears it.
	m.applyResponse(&tabResponse{status: "200 OK", rawBody: "{}"})
	if m.errorBanner.Visible() {
		t.Error("error banner not cleared after a successful response")
	}

	// Re-raise then dismiss.
	m.applyResponse(&tabResponse{isError: true, errText: "boom", status: "Error: boom"})
	if !m.errorBanner.Visible() {
		t.Fatal("banner not re-shown")
	}
	m.hideErrorBanner()
	if m.errorBanner.Visible() {
		t.Error("dismiss did not hide the banner")
	}
}
