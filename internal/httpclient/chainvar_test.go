package httpclient

import (
	"context"
	"testing"

	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestBuildResolvesChainVarInBearer verifies a {{chain.<alias>.response.json.*}}
// template in the Bearer token resolves end-to-end: Build -> auth.ResolveValues
// -> auth.Apply sets the resolved token on the Authorization header.
func TestBuildResolvesChainVarInBearer(t *testing.T) {
	views := map[string]chain.View{
		"login": {Response: chain.ResponseView{Body: []byte(`{"token":"abc123"}`)}},
	}
	res := vars.New().WithFallback(chain.VarLookup(views))
	req := model.Request{
		Method: model.GET,
		URL:    "https://api.example.com/me",
		Auth: model.Auth{
			Type:   model.AuthBearer,
			Bearer: &model.BearerAuth{Token: "{{chain.login.response.json.token}}"},
		},
	}
	httpReq, _, err := Build(context.Background(), req, res, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer abc123")
	}
}

// TestBuildUnresolvedChainVarErrors verifies an unresolvable chain var fails the
// build with the unresolved-variables error (the user sees the bad path).
func TestBuildUnresolvedChainVarErrors(t *testing.T) {
	res := vars.New().WithFallback(chain.VarLookup(map[string]chain.View{}))
	req := model.Request{Method: model.GET, URL: "https://x/{{chain.login.response.json.token}}"}
	if _, _, err := Build(context.Background(), req, res, nil); err == nil {
		t.Fatal("Build with unresolved chain var: err=nil, want an unresolved-variables error")
	}
}
