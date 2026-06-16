package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestHelpMenuOffersMoreThanShortcuts verifies the Help menu includes a
// getting-started entry, web links, and About — not just the keymap (#61).
func TestHelpMenuOffersMoreThanShortcuts(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	items := m.helpMenuItems()
	labels := map[string]bool{}
	for _, it := range items {
		if !it.IsSeparator {
			labels[it.Label] = true
		}
	}
	for _, want := range []string{"Getting started", "Keyboard shortcuts", "User guide (web)", "Report an issue (web)", "About Helena"} {
		if !labels[want] {
			t.Errorf("Help menu missing %q (have %v)", want, labels)
		}
	}
	if len(labels) <= 1 {
		t.Error("Help menu offers only the keymap")
	}
}

// TestAboutUsesSetVersion verifies SetVersion feeds the About entry. We can't
// assert dialog contents headlessly, but we can pin the stored version and that
// showAbout is a no-op without a window (no panic).
func TestAboutUsesSetVersion(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	m.SetVersion("v9.9.9")
	if m.appVersion != "v9.9.9" {
		t.Errorf("appVersion = %q, want v9.9.9", m.appVersion)
	}
	m.showAbout() // window-less: must not panic
}

// TestHelpURLsWellFormed guards the hard-coded help links.
func TestHelpURLsWellFormed(t *testing.T) {
	for _, u := range []string{repoURL, userGuideURL, issuesURL} {
		if !strings.HasPrefix(u, "https://github.com/ideaconnect/helena") {
			t.Errorf("unexpected help URL %q", u)
		}
	}
}
