package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestBodyFormEditorLoadsAndToggles verifies the Body.Form KV editor is shown
// for form-urlencoded/multipart, populated from the request, and hidden for raw
// body types (regression for #22).
func TestBodyFormEditorLoadsAndToggles(t *testing.T) {
	m := newResponseUI(t)

	// form-urlencoded with two fields: the form table is shown and populated,
	// the raw text editor hidden.
	req := &model.Request{Body: model.Body{Type: model.BodyForm, Form: []model.KeyValue{
		{Enabled: true, Key: "a", Value: "1"},
		{Enabled: true, Key: "b", Value: "2"},
	}}}
	m.loadRequest(req, "id-1")
	if got := len(m.bodyFormRows.Objects); got != 2 {
		t.Errorf("form rows = %d, want 2", got)
	}
	if m.bodyFormPanel.Hidden {
		t.Error("form panel hidden for form-urlencoded; want shown")
	}
	if !m.BodyContent.Hidden {
		t.Error("raw text editor shown for form-urlencoded; want hidden")
	}

	// Switch to JSON: the raw editor returns, the form table hides.
	m.BodyType.SetSelected(string(model.BodyJSON))
	if !m.bodyFormPanel.Hidden {
		t.Error("form panel shown for JSON; want hidden")
	}
	if m.BodyContent.Hidden {
		t.Error("raw text editor hidden for JSON; want shown")
	}
}

// TestAddBodyFormFieldWritesModel verifies the editor's add affordance grows
// the request's Body.Form (the field the send path uses) (#22).
func TestAddBodyFormFieldWritesModel(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Body: model.Body{Type: model.BodyForm}}
	m.loadRequest(req, "id-1")

	m.addBodyFormField()
	if len(req.Body.Form) != 1 {
		t.Fatalf("Body.Form len = %d, want 1 after add", len(req.Body.Form))
	}
	if len(m.bodyFormRows.Objects) != 1 {
		t.Errorf("form rows = %d, want 1", len(m.bodyFormRows.Objects))
	}
}
