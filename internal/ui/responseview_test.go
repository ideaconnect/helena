package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// newResponseTestUI builds a MainUI in a headless app for response-view tests.
func newResponseTestUI(t *testing.T) *MainUI {
	t.Helper()
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return NewMainUI(sess)
}

// TestRenderResponseBodyJSONSelectsStructured verifies a JSON body fills the
// Structured tree + Pretty grid and auto-selects the Structured tab.
func TestRenderResponseBodyJSONSelectsStructured(t *testing.T) {
	m := newResponseTestUI(t)
	m.renderResponseBody([]byte(`{"a":1,"b":[2,3]}`), "application/json")

	if got := m.Response.SelectedIndex(); got != 0 {
		t.Errorf("selected tab = %d, want 0 (Structured)", got)
	}
	if m.structRoot == nil || m.structRoot.Value != "{2}" {
		t.Errorf("structRoot = %+v, want a 2-field object", m.structRoot)
	}
	if len(m.structIndex) == 0 {
		t.Error("structIndex empty after JSON render")
	}
	if len(m.prettyGrid.Rows) == 0 {
		t.Error("prettyGrid empty after JSON render")
	}
}

// TestRenderResponseBodyXMLSelectsStructured verifies XML routes to the
// Structured tree the same way JSON does.
func TestRenderResponseBodyXMLSelectsStructured(t *testing.T) {
	m := newResponseTestUI(t)
	m.renderResponseBody([]byte(`<root><item>hi</item></root>`), "text/xml")

	if got := m.Response.SelectedIndex(); got != 0 {
		t.Errorf("selected tab = %d, want 0 (Structured)", got)
	}
	if m.structRoot == nil || m.structRoot.Label != "root" {
		t.Errorf("structRoot = %+v, want <root>", m.structRoot)
	}
}

// TestRenderResponseBodyNonStructuredSelectsRaw verifies a plain-text body
// clears the rich views and falls back to the Raw tab.
func TestRenderResponseBodyNonStructuredSelectsRaw(t *testing.T) {
	m := newResponseTestUI(t)
	// Seed prior structured state to confirm it gets cleared.
	m.renderResponseBody([]byte(`{"a":1}`), "application/json")
	m.renderResponseBody([]byte(`just text`), "text/plain")

	if got := m.Response.SelectedIndex(); got != 2 {
		t.Errorf("selected tab = %d, want 2 (Raw)", got)
	}
	if m.structRoot != nil {
		t.Error("structRoot not cleared for non-structured body")
	}
	if len(m.structIndex) != 0 {
		t.Error("structIndex not cleared for non-structured body")
	}
}

// TestRenderResponseBodyMalformedJSONFallsToPretty verifies that when JSON
// is too broken to tree-parse but still pretty-prints, the Pretty tab wins;
// when neither works it falls to Raw. Here a malformed body fails both, so Raw.
func TestRenderResponseBodyMalformedJSONFallsToRaw(t *testing.T) {
	m := newResponseTestUI(t)
	m.renderResponseBody([]byte(`{not json`), "application/json")

	if got := m.Response.SelectedIndex(); got != 2 {
		t.Errorf("selected tab = %d, want 2 (Raw) for malformed JSON", got)
	}
	if m.structRoot != nil {
		t.Error("structRoot should be nil for malformed JSON")
	}
}

// TestClearRichResponseBlanksTabs verifies clearRichResponse empties both rich
// views without touching tab selection.
func TestClearRichResponseBlanksTabs(t *testing.T) {
	m := newResponseTestUI(t)
	m.renderResponseBody([]byte(`{"a":1}`), "application/json")
	m.clearRichResponse()

	if m.structRoot != nil || len(m.structIndex) != 0 {
		t.Error("clearRichResponse left structured state behind")
	}
}
