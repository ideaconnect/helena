package ui

import (
	"errors"
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestNewSnippetEntryIsReadOnly verifies export snippet entries are disabled so
// the generated command can't be accidentally edited before copying (#23). The
// Copy buttons read .Text directly, so copy still works.
func TestNewSnippetEntryIsReadOnly(t *testing.T) {
	test.NewApp()

	e := newSnippetEntry("curl https://example.test", nil)
	if !e.Disabled() {
		t.Error("snippet entry is editable; want read-only (Disabled)")
	}
	if e.Text != "curl https://example.test" {
		t.Errorf("snippet text = %q", e.Text)
	}

	errEntry := newSnippetEntry("", errors.New("boom"))
	if !errEntry.Disabled() {
		t.Error("error snippet entry is editable; want read-only")
	}
	if errEntry.Text != "# error: boom" {
		t.Errorf("error snippet text = %q", errEntry.Text)
	}
}
