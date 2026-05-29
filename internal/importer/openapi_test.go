package importer

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

const oas3Sample = `openapi: 3.0.0
info:
  title: Sample API
servers:
  - url: https://api.example.com/v1
paths:
  /users:
    get:
      summary: List users
      tags: [users]
      parameters:
        - in: query
          name: limit
          required: false
          schema:
            type: integer
            default: 10
    post:
      summary: Create user
      tags: [users]
      requestBody:
        content:
          application/json:
            example:
              name: Alice
  /health:
    get:
      summary: Health check
`

const swagger2Sample = `swagger: "2.0"
info:
  title: Legacy API
host: api.legacy.com
basePath: /v1
schemes:
  - https
paths:
  /ping:
    get:
      summary: Ping
      tags:
        - ops
      parameters:
        - in: query
          name: verbose
          required: false
          type: boolean
`

// TestFromOpenAPI3 verifies a full OpenAPI 3 YAML spec maps to a collection with base_url env, tag-folders, request bodies and disabled optional params.
func TestFromOpenAPI3(t *testing.T) {
	c, err := FromOpenAPI([]byte(oas3Sample))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if c.Name != "Sample API" {
		t.Errorf("collection name = %q", c.Name)
	}
	// Default environment with base_url.
	if len(c.Environments) != 1 || c.Environments[0].Name != "Default" {
		t.Fatalf("environments = %+v", c.Environments)
	}
	v := c.Environments[0].Variables
	if len(v) != 1 || v[0].Key != "base_url" || v[0].Value != "https://api.example.com/v1" {
		t.Errorf("base_url variable = %+v", v)
	}
	// One folder "users" with two requests, plus one root request "Health check".
	if len(c.Folders) != 1 || c.Folders[0].Name != "users" {
		t.Fatalf("folders = %+v", c.Folders)
	}
	if len(c.Folders[0].Requests) != 2 {
		t.Errorf("users folder requests = %d, want 2", len(c.Folders[0].Requests))
	}
	if len(c.Requests) != 1 || c.Requests[0].Name != "Health check" {
		t.Errorf("root requests = %+v", c.Requests)
	}

	// Look at the POST /users specifically — it should have the example body.
	var post *model.Request
	for i := range c.Folders[0].Requests {
		if c.Folders[0].Requests[i].Method == model.POST {
			post = &c.Folders[0].Requests[i]
		}
	}
	if post == nil {
		t.Fatalf("POST /users not found")
	}
	if post.URL != "{{base_url}}/users" {
		t.Errorf("POST URL = %q", post.URL)
	}
	if post.Body.Type != model.BodyJSON {
		t.Errorf("POST body type = %q", post.Body.Type)
	}
	if !strings.Contains(post.Body.Content, `"name"`) || !strings.Contains(post.Body.Content, "Alice") {
		t.Errorf("POST body content = %q", post.Body.Content)
	}

	// The GET /users should carry the optional `limit` query param (disabled).
	var get *model.Request
	for i := range c.Folders[0].Requests {
		if c.Folders[0].Requests[i].Method == model.GET {
			get = &c.Folders[0].Requests[i]
		}
	}
	if get == nil || len(get.Params) != 1 {
		t.Fatalf("GET /users params = %+v", get)
	}
	if get.Params[0].Key != "limit" || get.Params[0].Enabled {
		t.Errorf("limit param = %+v (want disabled, key=limit)", get.Params[0])
	}
}

// TestFromSwagger2 verifies a Swagger 2 spec is converted via openapi2conv, preserving title, host+basePath as base_url, and tag-folder grouping.
func TestFromSwagger2(t *testing.T) {
	c, err := FromOpenAPI([]byte(swagger2Sample))
	if err != nil {
		t.Fatalf("FromOpenAPI (swagger 2): %v", err)
	}
	if c.Name != "Legacy API" {
		t.Errorf("name = %q", c.Name)
	}
	if len(c.Environments) != 1 {
		t.Fatalf("env = %+v", c.Environments)
	}
	got := c.Environments[0].Variables[0].Value
	if got != "https://api.legacy.com/v1" {
		t.Errorf("base_url = %q, want https://api.legacy.com/v1", got)
	}
	if len(c.Folders) != 1 || c.Folders[0].Name != "ops" {
		t.Fatalf("folders = %+v", c.Folders)
	}
	if len(c.Folders[0].Requests) != 1 || c.Folders[0].Requests[0].Method != model.GET {
		t.Errorf("ops folder request = %+v", c.Folders[0].Requests)
	}
}

// TestFromOpenAPIRejectsNonSpec verifies a JSON document with neither openapi nor swagger key returns an error.
func TestFromOpenAPIRejectsNonSpec(t *testing.T) {
	_, err := FromOpenAPI([]byte(`{"hello": "world"}`))
	if err == nil {
		t.Fatalf("expected error for non-OpenAPI doc")
	}
}

// TestRequestNameFallbacks verifies the three name sources for a
// generated request: Summary > OperationID > "METHOD path". Mirrors
// the chain the importer documents.
func TestRequestNameFallbacks(t *testing.T) {
	const spec = `openapi: 3.0.0
info:
  title: X
paths:
  /a:
    get:
      summary: Has summary
  /b:
    get:
      operationId: getB
  /c:
    get: {}
`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	got := map[string]bool{}
	for _, r := range c.Requests {
		got[r.Name] = true
	}
	want := []string{"Has summary", "getB", "GET /c"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing request name %q in %v", w, got)
		}
	}
}

// TestBodyTypeFromContentTypeBranches verifies the content-type sniff
// covers every BodyType branch the importer exposes.
func TestBodyTypeFromContentTypeBranches(t *testing.T) {
	cases := map[string]model.BodyType{
		"application/json":                  model.BodyJSON,
		"application/xml":                   model.BodyXML,
		"application/x-www-form-urlencoded": model.BodyForm,
		"multipart/form-data":               model.BodyMultipart,
		"text/plain":                        model.BodyText,
		"text/csv":                          model.BodyText,
		"application/octet-stream":          model.BodyText, // default
	}
	for ct, want := range cases {
		if got := bodyTypeFromContentType(ct); got != want {
			t.Errorf("bodyTypeFromContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

// TestExtractExampleFallbacks verifies that extractExample picks an
// example from each of the three places OpenAPI allows: the MediaType
// example directly, the Examples map, and the Schema's example.
func TestExtractExampleFallbacks(t *testing.T) {
	const spec = `openapi: 3.0.0
info:
  title: X
paths:
  /examples-map:
    post:
      requestBody:
        content:
          application/json:
            examples:
              first:
                value:
                  via: examples-map
  /schema-example:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              example:
                via: schema
  /no-example:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	bodies := map[string]string{}
	for _, r := range c.Requests {
		bodies[r.URL] = r.Body.Content
	}
	if !strings.Contains(bodies["/examples-map"], "examples-map") {
		t.Errorf("examples-map body = %q", bodies["/examples-map"])
	}
	if !strings.Contains(bodies["/schema-example"], "schema") {
		t.Errorf("schema-example body = %q", bodies["/schema-example"])
	}
	if bodies["/no-example"] != "" {
		t.Errorf("no-example body = %q, want empty", bodies["/no-example"])
	}
}

// TestDefaultParamValueFromSchema verifies that defaultParamValue
// falls back to the parameter's Schema.Default when the parameter
// itself carries no Example. Matches how OpenAPI authors typically
// declare default param values.
func TestDefaultParamValueFromSchema(t *testing.T) {
	const spec = `openapi: 3.0.0
info:
  title: X
paths:
  /list:
    get:
      parameters:
        - in: query
          name: page
          required: false
          schema:
            type: integer
            default: 1
`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if len(c.Requests) != 1 || len(c.Requests[0].Params) != 1 {
		t.Fatalf("requests = %+v", c.Requests)
	}
	if c.Requests[0].Params[0].Value != "1" {
		t.Errorf("page default = %q, want 1", c.Requests[0].Params[0].Value)
	}
}

// TestHeaderParamLandsInHeaders verifies an OpenAPI parameter with
// in=header lands on the request's Headers rather than Params.
func TestHeaderParamLandsInHeaders(t *testing.T) {
	const spec = `openapi: 3.0.0
info:
  title: X
paths:
  /x:
    get:
      parameters:
        - in: header
          name: X-Trace
          required: true
`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	r := c.Requests[0]
	if len(r.Headers) != 1 || r.Headers[0].Key != "X-Trace" || !r.Headers[0].Enabled {
		t.Errorf("headers = %+v, want enabled X-Trace", r.Headers)
	}
	if len(r.Params) != 0 {
		t.Errorf("params = %+v, want empty (header param shouldn't double-up)", r.Params)
	}
}

// TestFromOpenAPIInvalidYAMLReturnsError verifies that a malformed
// YAML input doesn't crash and returns a clear parse error.
func TestFromOpenAPIInvalidYAMLReturnsError(t *testing.T) {
	_, err := FromOpenAPI([]byte("openapi: 3.0.0\npaths:\n  /x:\n    get:\n      summary: [unterminated"))
	if err == nil {
		t.Error("expected parse error from malformed YAML")
	}
}

// TestLooksLikeXMLBranches verifies the XML sniffer's three observable
// outcomes: bytes starting with '<' (after leading whitespace) are
// XML; bytes starting with anything else are not; an all-whitespace
// or empty input is not.
func TestLooksLikeXMLBranches(t *testing.T) {
	cases := map[string]bool{
		"<wsdl>":       true,
		"  \n\t<wsdl>": true,
		"{\"json\":1}": false,
		"openapi: 3.0": false,
		"":             false,
		"   \t\n":      false,
	}
	for in, want := range cases {
		if got := looksLikeXML([]byte(in)); got != want {
			t.Errorf("looksLikeXML(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestFromOpenAPIJSONInput verifies that raw JSON specs skip YAML normalization and parse directly.
func TestFromOpenAPIJSONInput(t *testing.T) {
	// Same spec but in JSON form.
	in := `{"openapi":"3.0.0","info":{"title":"JSON API"},"paths":{}}`
	c, err := FromOpenAPI([]byte(in))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if c.Name != "JSON API" {
		t.Errorf("name = %q", c.Name)
	}
}
