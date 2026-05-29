package ui

import (
	"runtime"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestRegisterShortcutsPopulatesList verifies that registerShortcuts fills
// m.shortcuts with non-nil actions and covers each of the documented bindings
// (Send, Save, Open, Import, New request, New collection, Settings).
func TestRegisterShortcutsPopulatesList(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	if len(m.shortcuts) == 0 {
		t.Fatal("expected shortcuts to be registered")
	}

	// Sanity-check that the expected actions are present.
	wantActions := []string{
		"Send request",
		"Save request",
		"Open collection",
		"Import",
		"New request",
		"New collection",
		"Settings",
	}
	have := map[string]bool{}
	for _, s := range m.shortcuts {
		if s.do == nil {
			t.Errorf("shortcut %q has no action", s.action)
		}
		have[s.action] = true
	}
	for _, want := range wantActions {
		found := false
		for action := range have {
			if strings.Contains(action, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing shortcut for action containing %q", want)
		}
	}
}

// TestShortcutModifierName verifies that the platform modifier label is the
// Command symbol on macOS and "Ctrl" everywhere else.
func TestShortcutModifierName(t *testing.T) {
	got := shortcutModifierName()
	want := "Ctrl"
	if runtime.GOOS == "darwin" {
		want = "⌘"
	}
	if got != want {
		t.Errorf("shortcutModifierName() = %q on %s, want %q", got, runtime.GOOS, want)
	}
}

// TestShowShortcutsDoesNotPanic verifies that opening the shortcuts help
// dialog is safe when shortcuts have been registered against a real window.
func TestShowShortcutsDoesNotPanic(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	// Should be a no-op at worst — but must not panic.
	m.showShortcuts()
}

// TestRegisterShortcutsWithoutWindowIsNoop verifies that calling
// registerShortcuts and showShortcuts before SetWindow short-circuits safely
// instead of dereferencing the nil m.win.
func TestRegisterShortcutsWithoutWindowIsNoop(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	// No SetWindow call — registerShortcuts must short-circuit safely.
	m.registerShortcuts()
	if len(m.shortcuts) != 0 {
		t.Errorf("expected no shortcuts without window, got %d", len(m.shortcuts))
	}
	// Also ensure showShortcuts is safe when win is nil.
	m.showShortcuts()
}
