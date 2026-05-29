package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestDocsEditorLoadsAndPreviews verifies that loading a request copies its
// Docs into the editor, that refreshDocsPreview runs without panic, and that
// typing into the editor writes back into the in-memory request.
func TestDocsEditorLoadsAndPreviews(t *testing.T) {
	test.NewApp()

	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name:   "Documented",
		Method: model.GET,
		URL:    "https://example.com",
		Docs:   "# Hello\n\n- one\n- two\n",
	}
	m.loadRequest(req, "0/r0")
	if m.docsEditor.Text != req.Docs {
		t.Errorf("docsEditor.Text = %q, want %q", m.docsEditor.Text, req.Docs)
	}

	// Refresh the preview the same way the tab-switch handler would.
	m.refreshDocsPreview()

	// Typing into the editor must write back to the in-memory request.
	m.docsEditor.OnChanged("changed")
	if m.currentRequest.Docs != "changed" {
		t.Errorf("write-back: Docs = %q, want %q", m.currentRequest.Docs, "changed")
	}
}

// TestDocsEditorClearsOnNilRequest verifies that loadRequest(nil, "") empties
// the docsEditor so a previously loaded request's Markdown doesn't stay
// visible after the selection is cleared.
func TestDocsEditorClearsOnNilRequest(t *testing.T) {
	test.NewApp()

	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	m.loadRequest(&model.Request{Name: "R", Docs: "filled"}, "0/r0")
	if m.docsEditor.Text != "filled" {
		t.Fatalf("setup: docsEditor = %q", m.docsEditor.Text)
	}
	m.loadRequest(nil, "")
	if m.docsEditor.Text != "" {
		t.Errorf("after clear: docsEditor = %q, want empty", m.docsEditor.Text)
	}
}
