package importer

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// samplePostman is a representative Postman v2.1 export exercising folders,
// nested requests, a string URL, an object URL with query, disabled headers,
// raw-json / urlencoded / graphql bodies, and bearer auth.
const samplePostman = `{
  "info": { "name": "Sample API", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json" },
  "auth": { "type": "bearer", "bearer": [{ "key": "token", "value": "abc123" }] },
  "item": [
    {
      "name": "Health",
      "request": { "method": "GET", "url": "https://api.example.com/health" }
    },
    {
      "name": "Users",
      "item": [
        {
          "name": "List users",
          "request": {
            "method": "GET",
            "header": [
              { "key": "Accept", "value": "application/json" },
              { "key": "X-Debug", "value": "1", "disabled": true }
            ],
            "url": {
              "raw": "https://api.example.com/users?page=2",
              "host": ["api","example","com"],
              "path": ["users"],
              "query": [{ "key": "page", "value": "2" }]
            }
          }
        },
        {
          "name": "Create user",
          "request": {
            "method": "POST",
            "url": "https://api.example.com/users",
            "body": { "mode": "raw", "raw": "{\"name\":\"a\"}", "options": { "raw": { "language": "json" } } },
            "auth": { "type": "basic", "basic": [
              { "key": "username", "value": "u" },
              { "key": "password", "value": "p" }
            ] }
          }
        }
      ]
    }
  ]
}`

// TestFromPostmanBuildsTree verifies folders, nested requests, the collection
// name, and collection-level auth map correctly.
func TestFromPostmanBuildsTree(t *testing.T) {
	c, err := FromPostman([]byte(samplePostman))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	if c.Name != "Sample API" {
		t.Errorf("name = %q, want Sample API", c.Name)
	}
	if c.Auth.Type != model.AuthBearer || c.Auth.Bearer == nil || c.Auth.Bearer.Token != "abc123" {
		t.Errorf("collection auth = %+v, want bearer abc123", c.Auth)
	}
	if len(c.Requests) != 1 || c.Requests[0].Name != "Health" {
		t.Fatalf("top-level requests = %+v, want [Health]", c.Requests)
	}
	if c.Requests[0].Method != model.GET || c.Requests[0].URL != "https://api.example.com/health" {
		t.Errorf("Health request = %+v", c.Requests[0])
	}
	if len(c.Folders) != 1 || c.Folders[0].Name != "Users" {
		t.Fatalf("folders = %+v, want [Users]", c.Folders)
	}
	if len(c.Folders[0].Requests) != 2 {
		t.Fatalf("Users requests = %d, want 2", len(c.Folders[0].Requests))
	}
}

// TestFromPostmanRequestDetails verifies header disabled-flag mapping, query
// params from an object URL, raw-json body typing, and per-request basic auth.
func TestFromPostmanRequestDetails(t *testing.T) {
	c, err := FromPostman([]byte(samplePostman))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	list := c.Folders[0].Requests[0] // List users
	if len(list.Headers) != 2 {
		t.Fatalf("headers = %d, want 2", len(list.Headers))
	}
	if !list.Headers[0].Enabled || list.Headers[1].Enabled {
		t.Errorf("header enabled flags = %v/%v, want true/false", list.Headers[0].Enabled, list.Headers[1].Enabled)
	}
	if len(list.Params) != 1 || list.Params[0].Key != "page" || list.Params[0].Value != "2" {
		t.Errorf("params = %+v, want [page=2]", list.Params)
	}

	create := c.Folders[0].Requests[1] // Create user
	if create.Method != model.POST {
		t.Errorf("method = %q, want POST", create.Method)
	}
	if create.Body.Type != model.BodyJSON || create.Body.Content != `{"name":"a"}` {
		t.Errorf("body = %+v, want json {\"name\":\"a\"}", create.Body)
	}
	if create.Auth.Type != model.AuthBasic || create.Auth.Basic == nil ||
		create.Auth.Basic.Username != "u" || create.Auth.Basic.Password != "p" {
		t.Errorf("auth = %+v, want basic u/p", create.Auth)
	}
}

// TestFromPostmanBodyModes verifies urlencoded, formdata, graphql, and apikey
// mappings that the shared sample doesn't cover.
func TestFromPostmanBodyModes(t *testing.T) {
	doc := `{
      "info": { "name": "Bodies" },
      "item": [
        { "name": "form", "request": { "method": "POST", "url": "x",
          "body": { "mode": "urlencoded", "urlencoded": [{ "key": "a", "value": "1" }, { "key": "b", "value": "2", "disabled": true }] } } },
        { "name": "multi", "request": { "method": "POST", "url": "x",
          "body": { "mode": "formdata", "formdata": [{ "key": "f", "value": "v" }] } } },
        { "name": "gql", "request": { "method": "POST", "url": "x",
          "body": { "mode": "graphql", "graphql": { "query": "{ me }", "variables": "{\"id\":1}" } } },
          "request_auth_placeholder": true },
        { "name": "key", "request": { "method": "GET", "url": "x",
          "auth": { "type": "apikey", "apikey": [{ "key": "key", "value": "X-Api" }, { "key": "value", "value": "secret" }, { "key": "in", "value": "query" }] } } }
      ]
    }`
	c, err := FromPostman([]byte(doc))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	form := c.Requests[0].Body
	if form.Type != model.BodyForm || len(form.Form) != 2 || form.Form[1].Enabled {
		t.Errorf("urlencoded body = %+v", form)
	}
	multi := c.Requests[1].Body
	if multi.Type != model.BodyMultipart || len(multi.Form) != 1 {
		t.Errorf("formdata body = %+v", multi)
	}
	gql := c.Requests[2].Body
	if gql.Type != model.BodyJSON || gql.Content == "" {
		t.Errorf("graphql body = %+v, want json", gql)
	}
	key := c.Requests[3].Auth
	if key.Type != model.AuthAPIKey || key.APIKey == nil ||
		key.APIKey.Name != "X-Api" || key.APIKey.Value != "secret" || key.APIKey.Placement != model.APIKeyQuery {
		t.Errorf("apikey auth = %+v", key)
	}
}

// TestFromPostmanReconstructsURL verifies an object URL with no raw field is
// rebuilt from host + path parts.
func TestFromPostmanReconstructsURL(t *testing.T) {
	doc := `{"info":{"name":"n"},"item":[{"name":"r","request":{"method":"GET","url":{"host":["api","x","com"],"path":["v1","ping"]}}}]}`
	c, err := FromPostman([]byte(doc))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	if got := c.Requests[0].URL; got != "api.x.com/v1/ping" {
		t.Errorf("reconstructed URL = %q, want api.x.com/v1/ping", got)
	}
}

// TestFromPostmanNoauthAndDefaults verifies noauth maps to explicit None and an
// empty method defaults to GET.
func TestFromPostmanNoauthAndDefaults(t *testing.T) {
	doc := `{"info":{"name":"n"},"item":[{"name":"r","request":{"url":"x","auth":{"type":"noauth"}}}]}`
	c, err := FromPostman([]byte(doc))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	if c.Requests[0].Method != model.GET {
		t.Errorf("method = %q, want GET default", c.Requests[0].Method)
	}
	if c.Requests[0].Auth.Type != model.AuthNone {
		t.Errorf("auth = %q, want none", c.Requests[0].Auth.Type)
	}
}

// TestFromPostmanInvalid verifies malformed JSON is rejected.
func TestFromPostmanInvalid(t *testing.T) {
	if _, err := FromPostman([]byte(`{not json`)); err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// TestLooksLikePostmanDetection verifies the detector accepts Postman docs and
// rejects OpenAPI/Swagger and non-collection JSON.
func TestLooksLikePostmanDetection(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"postman", `{"info":{"name":"x"},"item":[]}`, true},
		{"openapi", `{"openapi":"3.0.0","info":{"title":"x"},"item":[]}`, false},
		{"swagger", `{"swagger":"2.0","info":{"title":"x"}}`, false},
		{"no-item", `{"info":{"name":"x"}}`, false},
		{"garbage", `not json`, false},
	}
	for _, c := range cases {
		if got := looksLikePostman([]byte(c.data)); got != c.want {
			t.Errorf("%s: looksLikePostman = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFromPostmanBodyEdgeCases verifies xml raw bodies, a graphql body without
// variables, and unknown/file body modes mapping to no body.
func TestFromPostmanBodyEdgeCases(t *testing.T) {
	doc := `{"info":{"name":"n"},"item":[
      {"name":"xml","request":{"method":"POST","url":"x","body":{"mode":"raw","raw":"<a/>","options":{"raw":{"language":"xml"}}}}},
      {"name":"gqlNoVars","request":{"method":"POST","url":"x","body":{"mode":"graphql","graphql":{"query":"{ ping }"}}}},
      {"name":"gqlNil","request":{"method":"POST","url":"x","body":{"mode":"graphql"}}},
      {"name":"file","request":{"method":"POST","url":"x","body":{"mode":"file"}}}
    ]}`
	c, err := FromPostman([]byte(doc))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	if c.Requests[0].Body.Type != model.BodyXML {
		t.Errorf("xml body type = %q, want xml", c.Requests[0].Body.Type)
	}
	if c.Requests[1].Body.Type != model.BodyJSON {
		t.Errorf("graphql-no-vars body type = %q, want json", c.Requests[1].Body.Type)
	}
	if c.Requests[2].Body.Type != model.BodyNone {
		t.Errorf("graphql-nil body type = %q, want none", c.Requests[2].Body.Type)
	}
	if c.Requests[3].Body.Type != model.BodyNone {
		t.Errorf("file body type = %q, want none", c.Requests[3].Body.Type)
	}
}

// TestFromPostmanAuthEdgeCases verifies an unknown auth type falls through to
// the zero (Inherit) Auth and that non-string / missing auth values are handled.
func TestFromPostmanAuthEdgeCases(t *testing.T) {
	doc := `{"info":{"name":"n"},"item":[
      {"name":"oauth","request":{"method":"GET","url":"x","auth":{"type":"oauth2"}}},
      {"name":"numtoken","request":{"method":"GET","url":"x","auth":{"type":"bearer","bearer":[{"key":"token","value":42}]}}},
      {"name":"missing","request":{"method":"GET","url":"x","auth":{"type":"bearer","bearer":[{"key":"other","value":"z"}]}}}
    ]}`
	c, err := FromPostman([]byte(doc))
	if err != nil {
		t.Fatalf("FromPostman: %v", err)
	}
	if c.Requests[0].Auth.Type != "" {
		t.Errorf("oauth2 (unsupported) auth = %q, want zero/inherit", c.Requests[0].Auth.Type)
	}
	if c.Requests[1].Auth.Bearer.Token != "42" {
		t.Errorf("numeric token = %q, want stringified 42", c.Requests[1].Auth.Bearer.Token)
	}
	if c.Requests[2].Auth.Bearer.Token != "" {
		t.Errorf("missing token = %q, want empty", c.Requests[2].Auth.Bearer.Token)
	}
}

// TestPMURLReconstructionVariants verifies effectiveRaw's host-only, path-only,
// and single-string host/path handling.
func TestPMURLReconstructionVariants(t *testing.T) {
	cases := []struct {
		name, doc, want string
	}{
		{"hostOnly", `{"info":{"name":"n"},"item":[{"name":"r","request":{"url":{"host":["api","x"]}}}]}`, "api.x"},
		{"pathOnly", `{"info":{"name":"n"},"item":[{"name":"r","request":{"url":{"path":["a","b"]}}}]}`, "a/b"},
		{"stringHost", `{"info":{"name":"n"},"item":[{"name":"r","request":{"url":{"host":"api.x.com","path":"p"}}}]}`, "api.x.com/p"},
		{"emptyURL", `{"info":{"name":"n"},"item":[{"name":"r","request":{"url":{}}}]}`, ""},
	}
	for _, tc := range cases {
		c, err := FromPostman([]byte(tc.doc))
		if err != nil {
			t.Fatalf("%s: FromPostman: %v", tc.name, err)
		}
		if got := c.Requests[0].URL; got != tc.want {
			t.Errorf("%s: URL = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFromRoutesPostman verifies the top-level From dispatcher routes a Postman
// document to FromPostman (not the OpenAPI parser).
func TestFromRoutesPostman(t *testing.T) {
	c, err := From([]byte(samplePostman))
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if c.Name != "Sample API" || len(c.Folders) != 1 {
		t.Errorf("From did not route to Postman importer: %+v", c)
	}
}
