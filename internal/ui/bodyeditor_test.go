package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/model"
)

// TestFormatForBodyType pins the body-type -> prettyview-format mapping that
// drives syntax highlighting in the editable body widget.
func TestFormatForBodyType(t *testing.T) {
	cases := map[model.BodyType]prettyview.Format{
		model.BodyJSON:      prettyview.FormatJSON,
		model.BodyXML:       prettyview.FormatXML,
		model.BodyText:      prettyview.FormatRaw,
		model.BodyForm:      prettyview.FormatRaw,
		model.BodyMultipart: prettyview.FormatRaw,
		model.BodyNone:      prettyview.FormatRaw,
		model.BodyType(""):  prettyview.FormatRaw,
	}
	for bt, want := range cases {
		if got := formatForBodyType(bt); got != want {
			t.Errorf("formatForBodyType(%q) = %v, want %v", bt, got, want)
		}
	}
}

// TestLoadRequestPopulatesBodyEditor verifies loadRequest feeds the body into the
// editable widget with the format matching the body type.
func TestLoadRequestPopulatesBodyEditor(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{
		Method: model.POST,
		URL:    "https://example.test",
		Body:   model.Body{Type: model.BodyJSON, Content: `{"a":1,"b":[2,3]}`},
	}
	m.loadRequest(req, "id-1")

	if got := string(m.BodyContent.Source()); got != req.Body.Content {
		t.Errorf("editor source = %q, want %q", got, req.Body.Content)
	}
	if m.BodyContent.Format() != prettyview.FormatJSON {
		t.Errorf("editor format = %v, want FormatJSON", m.BodyContent.Format())
	}
}

// TestSyncBodyFromEditorPullsLiveText verifies the authoritative body is read from
// the editor's live buffer (its OnChanged is debounced, so a stale model field
// must be refreshed synchronously before Save/Send).
func TestSyncBodyFromEditorPullsLiveText(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Body: model.Body{Type: model.BodyText, Content: "orig"}}
	m.loadRequest(req, "id-1")

	// Simulate the user changing the buffer without the debounced OnChanged having
	// fired yet: the model field still reads the old value.
	m.BodyContent.SetData([]byte("edited body"), prettyview.FormatRaw)
	if req.Body.Content != "orig" {
		t.Fatalf("precondition: model already updated (%q) — SetData should not fire OnChanged", req.Body.Content)
	}

	m.syncBodyFromEditor()
	if req.Body.Content != "edited body" {
		t.Errorf("after sync, body = %q, want %q", req.Body.Content, "edited body")
	}
}

// TestBodyTypeChangeReparsesEditor verifies switching the Type selector re-parses
// the editor at the new format so highlighting tracks the body type.
func TestBodyTypeChangeReparsesEditor(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Body: model.Body{Type: model.BodyText, Content: `{"a":1}`}}
	m.loadRequest(req, "id-1")
	if m.BodyContent.Format() != prettyview.FormatRaw {
		t.Fatalf("precondition format = %v, want FormatRaw", m.BodyContent.Format())
	}

	m.BodyType.SetSelected(string(model.BodyJSON))
	if m.BodyContent.Format() != prettyview.FormatJSON {
		t.Errorf("after type=json, editor format = %v, want FormatJSON", m.BodyContent.Format())
	}
	if req.Body.Type != model.BodyJSON {
		t.Errorf("model body type = %v, want json", req.Body.Type)
	}
}

// TestFormatBodyPrettifiesThroughEditor verifies Format pulls the live buffer,
// prettifies via responsefmt, and pushes the result back into the editor.
func TestFormatBodyPrettifiesThroughEditor(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Body: model.Body{Type: model.BodyJSON, Content: ""}}
	m.loadRequest(req, "id-1")

	// User typed compact JSON; the debounced write-back hasn't landed.
	m.BodyContent.SetData([]byte(`{"a":1,"b":2}`), prettyview.FormatJSON)
	m.formatBody()

	if !strings.Contains(req.Body.Content, "\n") {
		t.Errorf("formatted body not multi-line: %q", req.Body.Content)
	}
	if got := string(m.BodyContent.Source()); got != req.Body.Content {
		t.Errorf("editor source %q != model content %q", got, req.Body.Content)
	}
	if m.Status.Text != "Formatted" {
		t.Errorf("status = %q, want Formatted", m.Status.Text)
	}
}

// TestBodyEditorsCaptureTab pins the Tab-key behavior introduced by the
// go-fyne-pretty-view v2.3 bump: *PrettyView is now a fyne.Tabbable whose
// AcceptsTab() reports its editable flag, so Fyne routes Tab into the buffer
// (insert a literal tab) instead of moving focus. The editable request-body and
// GraphQL-variables editors must capture Tab; the read-only response viewer must
// not. This is a deliberate, user-visible consequence of the bump — there is no
// longer a keyboard focus-escape from these editors.
func TestBodyEditorsCaptureTab(t *testing.T) {
	m := newResponseUI(t)
	if !m.BodyContent.AcceptsTab() {
		t.Error("BodyContent.AcceptsTab() = false, want true (editable editor must capture Tab)")
	}
	if !m.bodyGraphQLVars.AcceptsTab() {
		t.Error("bodyGraphQLVars.AcceptsTab() = false, want true")
	}
	if m.pv.AcceptsTab() {
		t.Error("read-only response viewer pv.AcceptsTab() = true, want false (Tab should still move focus)")
	}

	// End-to-end: a Tab key on the focused editor inserts a literal tab at the caret.
	m.BodyContent.FocusGained()
	m.BodyContent.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if got := string(m.BodyContent.Source()); got != "\t" {
		t.Errorf("after Tab, editor source = %q, want %q", got, "\t")
	}
}

// TestClearRequestEmptiesBodyEditor verifies loading a nil request clears the
// editor buffer.
func TestClearRequestEmptiesBodyEditor(t *testing.T) {
	m := newResponseUI(t)
	m.loadRequest(&model.Request{Body: model.Body{Type: model.BodyText, Content: "stuff"}}, "id-1")
	m.loadRequest(nil, "")
	if got := string(m.BodyContent.Source()); got != "" {
		t.Errorf("editor source after clear = %q, want empty", got)
	}
}
