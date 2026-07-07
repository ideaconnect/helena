package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/assets"
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
	for _, want := range []string{"Getting started", "Keyboard shortcuts", "Website", "Report an issue", "Buy me a coffee", "About Helena"} {
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

// TestShowAboutWithWindow verifies the About dialog — which now carries Helena's
// photo and the tribute note — opens as a modal overlay when a window is set,
// and that the photo is actually embedded.
func TestShowAboutWithWindow(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	m.SetVersion("v1.2.3")
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	if len(assets.HelenaCat) == 0 {
		t.Fatal("embedded Helena photo is empty")
	}
	before := len(w.Canvas().Overlays().List())
	m.showAbout()
	if after := len(w.Canvas().Overlays().List()); after <= before {
		t.Fatalf("About dialog did not open (before=%d after=%d)", before, after)
	}
}

// TestHelpURLsWellFormed guards the hard-coded help links.
func TestHelpURLsWellFormed(t *testing.T) {
	for _, u := range []string{repoURL, issuesURL} {
		if !strings.HasPrefix(u, "https://github.com/ideaconnect/helena") {
			t.Errorf("unexpected github URL %q", u)
		}
	}
	if websiteURL != "https://idct.tech/helena" {
		t.Errorf("websiteURL = %q, want https://idct.tech/helena", websiteURL)
	}
	if coffeeURL != "https://buymeacoffee.com/idct" {
		t.Errorf("coffeeURL = %q, want https://buymeacoffee.com/idct", coffeeURL)
	}
}
