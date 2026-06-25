package storage

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestGraphQLBodyRoundTrip pins #70: a BodyGraphQL request's query (Content) and
// GraphQLVariables survive Save -> Load.
func TestGraphQLBodyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name:   "Q",
			Method: model.POST,
			URL:    "https://x/graphql",
			Body: model.Body{
				Type:             model.BodyGraphQL,
				Content:          "query($id:ID!){ user(id:$id){ name } }",
				GraphQLVariables: `{"id":"42"}`,
			},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := got.Requests[0].Body
	if b.Type != model.BodyGraphQL || b.Content != col.Requests[0].Body.Content || b.GraphQLVariables != `{"id":"42"}` {
		t.Errorf("graphql body did not round-trip: %+v", b)
	}
}
