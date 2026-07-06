package ui

import (
	"errors"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/updatecheck"
)

// TestCurrentVersionText verifies dev/empty builds render "dev build" and a
// released tag renders verbatim.
func TestCurrentVersionText(t *testing.T) {
	cases := map[string]string{"": "dev build", "dev": "dev build", "v0.4.0": "v0.4.0"}
	for in, want := range cases {
		if got := currentVersionText(in); got != want {
			t.Errorf("currentVersionText(%q)=%q want %q", in, got, want)
		}
	}
}

// TestStatusBarVersionSegment verifies the bottom bar shows the version and that
// SetVersion (called after construction, as main does) refreshes it.
func TestStatusBarVersionSegment(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	// appVersion is empty at construction, so the segment defaults to dev.
	if m.statusVersion == nil || m.statusVersion.Text != "dev build" {
		t.Fatalf("initial version segment = %v, want 'dev build'", m.statusVersion)
	}
	m.SetVersion("v0.4.0")
	if m.statusVersion.Text != "v0.4.0" {
		t.Errorf("after SetVersion, segment = %q, want v0.4.0", m.statusVersion.Text)
	}
}

// TestApplyUpdateResult verifies each check outcome maps to the right status
// text and Download-link visibility, and that the button is re-enabled.
func TestApplyUpdateResult(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	m.SetVersion("v0.4.0")

	cases := []struct {
		name     string
		rel      updatecheck.Release
		err      error
		wantText string
		wantLink bool
	}{
		{"error", updatecheck.Release{}, errors.New("boom"), "Update check failed", false},
		{"update", updatecheck.Release{Tag: "v0.5.0", HTMLURL: "https://example.test/r"}, nil, "Update available: v0.5.0", true},
		{"uptodate", updatecheck.Release{Tag: "v0.4.0"}, nil, "Up to date (v0.4.0)", false},
		{"ahead", updatecheck.Release{Tag: "v0.3.0"}, nil, "Ahead of latest (v0.3.0)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.updateCheckBtn.Disable()
			m.updateLink.Hide()
			m.applyUpdateResult(tc.rel, tc.err)

			if m.updateStatus.Text != tc.wantText {
				t.Errorf("status = %q, want %q", m.updateStatus.Text, tc.wantText)
			}
			if m.updateCheckBtn.Disabled() {
				t.Error("check button left disabled")
			}
			if m.updateLink.Visible() != tc.wantLink {
				t.Errorf("download link visible = %v, want %v", m.updateLink.Visible(), tc.wantLink)
			}
		})
	}
}

// TestApplyUpdateResultDevBuild verifies a dev build (no comparable version)
// still reports the latest release rather than a comparison verdict.
func TestApplyUpdateResultDevBuild(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess) // appVersion == "" (dev)

	m.applyUpdateResult(updatecheck.Release{Tag: "v0.4.0"}, nil)
	if m.updateStatus.Text != "Latest release: v0.4.0" {
		t.Errorf("dev-build status = %q, want 'Latest release: v0.4.0'", m.updateStatus.Text)
	}
}
