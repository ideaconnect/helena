package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestLoadRequestPopulatesGraphQLVariables verifies a BodyGraphQL request loads
// the query into BodyContent and the variables into the dedicated editor, and
// that the variables panel becomes visible (#70).
func TestLoadRequestPopulatesGraphQLVariables(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{
		Method: model.POST,
		Body:   model.Body{Type: model.BodyGraphQL, Content: "{ ping }", GraphQLVariables: `{"x":1}`},
	}
	m.loadRequest(req, "id-1")

	if got := string(m.BodyContent.Source()); got != "{ ping }" {
		t.Errorf("query editor = %q, want { ping }", got)
	}
	if got := string(m.bodyGraphQLVars.Source()); got != `{"x":1}` {
		t.Errorf("variables editor = %q, want {\"x\":1}", got)
	}
	if m.bodyGraphQLPanel.Hidden {
		t.Error("variables panel should be visible for graphql body")
	}
}

// TestGraphQLVariablesSyncAndHide verifies the debounced variables editor is
// pulled into the model by syncBodyFromEditor, and that switching away from
// graphql hides the variables panel.
func TestGraphQLVariablesSyncAndHide(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Body: model.Body{Type: model.BodyGraphQL, Content: "{ ping }"}}
	m.loadRequest(req, "id-1")

	m.bodyGraphQLVars.SetText(`{"id":"7"}`)
	m.syncBodyFromEditor()
	if req.Body.GraphQLVariables != `{"id":"7"}` {
		t.Errorf("variables not synced: %q", req.Body.GraphQLVariables)
	}

	m.BodyType.SetSelected(string(model.BodyJSON))
	if !m.bodyGraphQLPanel.Hidden {
		t.Error("variables panel should hide when leaving graphql body")
	}
}
