package httpclient

import (
	"context"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestBuildGraphQLEnvelope verifies a BodyGraphQL request is sent as a
// {"query":…,"variables":…} JSON envelope with {{vars}} resolved in both the
// query and the variables, advertised as application/json (#70).
func TestBuildGraphQLEnvelope(t *testing.T) {
	req := model.Request{
		Method: model.POST,
		URL:    "http://x/graphql",
		Body: model.Body{
			Type:             model.BodyGraphQL,
			Content:          "query($id:ID!){ user(id:$id){ name } }",
			GraphQLVariables: `{"id":"{{uid}}"}`,
		},
	}
	r, body, err := Build(context.Background(), req, vars.New(map[string]string{"uid": "u-7"}), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := `{"query":"query($id:ID!){ user(id:$id){ name } }","variables":{"id":"u-7"}}`
	if string(body) != want {
		t.Errorf("body =\n  %s\nwant\n  %s", body, want)
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

// TestBuildGraphQLNoVariables verifies a blank variables field omits the
// "variables" key entirely (query-only envelope).
func TestBuildGraphQLNoVariables(t *testing.T) {
	req := model.Request{
		Method: model.POST,
		URL:    "http://x/graphql",
		Body:   model.Body{Type: model.BodyGraphQL, Content: "{ ping }", GraphQLVariables: "  "},
	}
	_, body, err := Build(context.Background(), req, vars.New(nil), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if want := `{"query":"{ ping }"}`; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

// TestBuildGraphQLInvalidVariables verifies non-JSON variables surface an error
// rather than sending a malformed body.
func TestBuildGraphQLInvalidVariables(t *testing.T) {
	req := model.Request{
		Method: model.POST,
		URL:    "http://x/graphql",
		Body:   model.Body{Type: model.BodyGraphQL, Content: "{ ping }", GraphQLVariables: "{not json"},
	}
	_, _, err := Build(context.Background(), req, vars.New(nil), nil)
	if err == nil || !strings.Contains(err.Error(), "graphql variables") {
		t.Errorf("err = %v, want a graphql variables error", err)
	}
}
