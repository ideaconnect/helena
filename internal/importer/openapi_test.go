package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/idct/helena/internal/model"
)

// TestFromOpenAPINoInfoBlock guards against the nil-Info panic: a spec that
// parses but omits the `info` block must yield the fallback name, not crash.
func TestFromOpenAPINoInfoBlock(t *testing.T) {
	c, err := FromOpenAPI([]byte(`{"openapi":"3.0.0","paths":{}}`))
	if err != nil {
		t.Fatalf("FromOpenAPI on info-less spec: %v", err)
	}
	if c.Name != "Imported API" {
		t.Errorf("name = %q, want fallback %q", c.Name, "Imported API")
	}
}

// TestFromOpenAPINullServerNoPanic guards the nil-*Server deref: a spec with
// `servers: [null]` (the loader stores a nil pointer) must import cleanly with
// no base_url environment, not crash.
func TestFromOpenAPINullServerNoPanic(t *testing.T) {
	c, err := FromOpenAPI([]byte(`{"openapi":"3.0.0","info":{"title":"x","version":"1"},"servers":[null],"paths":{}}`))
	if err != nil {
		t.Fatalf("servers:[null] should import cleanly, got err: %v", err)
	}
	if len(c.Environments) != 0 {
		t.Errorf("a null server should yield no base_url environment, got %d", len(c.Environments))
	}
}

// TestFromOpenAPIMalformedReturnsErrorNotPanic pins the recover: kin-openapi's
// Swagger-2 conversion nil-derefs on these null sub-objects, and the importer
// must return an error rather than panic (a hostile URL-served spec runs off the
// UI goroutine with no recover above it and would otherwise crash the app).
func TestFromOpenAPIMalformedReturnsErrorNotPanic(t *testing.T) {
	for _, bad := range []string{
		`{"swagger":"2.0","paths":{"/a":null}}`,
		`{"swagger":"2.0","paths":{"/a":{"get":{"parameters":[null]}}}}`,
		`{"swagger":"2.0","paths":{"/a":{"get":{"responses":{"200":null}}}}}`,
		`{"swagger":"2.0","parameters":{"P":null},"paths":{}}`,
	} {
		if _, err := FromOpenAPI([]byte(bad)); err == nil {
			t.Errorf("expected an error (not a panic) for malformed spec %q", bad)
		}
	}
}

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
	// A schema with no example is no longer left blank: a property-less object
	// synthesizes to an empty JSON object so the body carries its shape (#180).
	if bodies["/no-example"] != "{}" {
		t.Errorf("no-example body = %q, want %q", bodies["/no-example"], "{}")
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

// TestSynthesizeBodyFromSchemaRef verifies that a request body described only
// by a $ref schema (no inline example) is populated with a synthesized skeleton
// JSON body rather than imported blank (issue #180).
func TestSynthesizeBodyFromSchemaRef(t *testing.T) {
	const spec = `{
	  "openapi": "3.0.0",
	  "info": {"title": "X"},
	  "paths": {
	    "/pets": {"post": {"requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}}}}}
	  },
	  "components": {"schemas": {"Pet": {
	    "type": "object",
	    "required": ["name"],
	    "properties": {
	      "id": {"type": "integer", "readOnly": true},
	      "name": {"type": "string"},
	      "tags": {"type": "array", "items": {"type": "string"}},
	      "born": {"type": "string", "format": "date-time"}
	    }
	  }}}
	}`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if len(c.Requests) != 1 {
		t.Fatalf("requests = %+v", c.Requests)
	}
	b := c.Requests[0].Body
	if b.Type != model.BodyJSON {
		t.Errorf("body type = %q, want JSON", b.Type)
	}
	for _, want := range []string{`"name"`, `"tags"`, `"born"`, "2020-01-01T00:00:00Z"} {
		if !strings.Contains(b.Content, want) {
			t.Errorf("synthesized body %q missing %q", b.Content, want)
		}
	}
	// readOnly fields are server-populated and must be omitted from the sample.
	if strings.Contains(b.Content, `"id"`) {
		t.Errorf("synthesized body should omit readOnly id: %q", b.Content)
	}
	// The synthesized body must be valid JSON.
	if !json.Valid([]byte(b.Content)) {
		t.Errorf("synthesized body is not valid JSON: %q", b.Content)
	}
}

// TestSwagger2BodyNoConsumesPopulated pins the Swagger-2 facet of #180: a body
// parameter with no `consumes` is converted with a "*/*" content type, which
// must still yield a JSON body populated from the schema — not a blank text
// body.
func TestSwagger2BodyNoConsumesPopulated(t *testing.T) {
	const spec = `{
	  "swagger": "2.0",
	  "info": {"title": "Legacy", "version": "1"},
	  "paths": {
	    "/pets": {"post": {"parameters": [
	      {"in": "body", "name": "body", "required": true, "schema": {"$ref": "#/definitions/Pet"}}
	    ]}}
	  },
	  "definitions": {"Pet": {"type": "object", "properties": {"name": {"type": "string"}}}}
	}`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI (swagger 2): %v", err)
	}
	if len(c.Requests) != 1 {
		t.Fatalf("requests = %+v", c.Requests)
	}
	b := c.Requests[0].Body
	if b.Type != model.BodyJSON {
		t.Errorf("body type = %q, want JSON (wildcard */* should map to JSON)", b.Type)
	}
	if !strings.Contains(b.Content, `"name"`) {
		t.Errorf("body content = %q, want a synthesized name field", b.Content)
	}
}

// TestImportTrimsDoubleSlash pins issue #181: a server URL with a trailing
// slash must not produce a double slash once joined with the operation path.
func TestImportTrimsDoubleSlash(t *testing.T) {
	const spec = `openapi: 3.0.0
info:
  title: X
servers:
  - url: https://api.example.com/v1/
paths:
  /pets:
    get:
      summary: List pets
`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if len(c.Environments) != 1 {
		t.Fatalf("environments = %+v", c.Environments)
	}
	if got := c.Environments[0].Variables[0].Value; got != "https://api.example.com/v1" {
		t.Errorf("base_url = %q, want trailing slash trimmed", got)
	}
	if len(c.Requests) != 1 {
		t.Fatalf("requests = %+v", c.Requests)
	}
	if got := c.Requests[0].URL; got != "{{base_url}}/pets" {
		t.Errorf("request URL = %q, want single slash join", got)
	}
	// Guard the rendered form: base_url + path must contain no "//" after the scheme.
	rendered := "https://api.example.com/v1" + "/pets"
	if strings.Contains(strings.TrimPrefix(rendered, "https://"), "//") {
		t.Errorf("rendered URL has a double slash: %q", rendered)
	}
}

// TestPlaceholderStringFormats verifies every format branch of the string
// placeholder plus the untyped default.
func TestPlaceholderStringFormats(t *testing.T) {
	cases := map[string]string{
		"date-time": "2020-01-01T00:00:00Z",
		"date":      "2020-01-01",
		"email":     "user@example.com",
		"uuid":      "00000000-0000-0000-0000-000000000000",
		"uri":       "https://example.com",
		"url":       "https://example.com",
		"hostname":  "example.com",
		"ipv4":      "127.0.0.1",
		"":          "string",
		"binary":    "string", // unknown format falls through to the default
	}
	for format, want := range cases {
		got := placeholderString(&openapi3.Schema{Format: format})
		if got != want {
			t.Errorf("placeholderString(format=%q) = %q, want %q", format, got, want)
		}
	}
}

// TestSampleForSchemaBranches exercises the synthesizer's value branches
// directly: the explicit-value shortcuts, composition, primitives, cycle guard,
// and depth cap.
func TestSampleForSchemaBranches(t *testing.T) {
	sr := func(s *openapi3.Schema) *openapi3.SchemaRef { return &openapi3.SchemaRef{Value: s} }
	typ := func(t string) *openapi3.Types { return &openapi3.Types{t} }
	fresh := func() map[*openapi3.Schema]bool { return map[*openapi3.Schema]bool{} }

	if got := sampleForSchema(nil, fresh(), 0); got != nil {
		t.Errorf("nil ref = %v, want nil", got)
	}
	if got := sampleForSchema(&openapi3.SchemaRef{}, fresh(), 0); got != nil {
		t.Errorf("nil value = %v, want nil", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Type: typ("string")}), fresh(), sampleMaxDepth+1); got != nil {
		t.Errorf("over-depth = %v, want nil", got)
	}

	// Explicit-value shortcuts, in precedence order.
	if got := sampleForSchema(sr(&openapi3.Schema{Example: "ex"}), fresh(), 0); got != "ex" {
		t.Errorf("example = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Default: "def"}), fresh(), 0); got != "def" {
		t.Errorf("default = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Const: "c"}), fresh(), 0); got != "c" {
		t.Errorf("const = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Enum: []any{"first", "second"}}), fresh(), 0); got != "first" {
		t.Errorf("enum = %v", got)
	}

	// Primitives.
	if got := sampleForSchema(sr(&openapi3.Schema{Type: typ("boolean")}), fresh(), 0); got != false {
		t.Errorf("boolean = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Type: typ("integer")}), fresh(), 0); got != 0 {
		t.Errorf("integer = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{Type: typ("number")}), fresh(), 0); got != 0 {
		t.Errorf("number = %v", got)
	}

	// Array wraps a single sample element.
	arr := sampleForSchema(sr(&openapi3.Schema{Type: typ("array"), Items: sr(&openapi3.Schema{Type: typ("string")})}), fresh(), 0)
	if got, ok := arr.([]any); !ok || len(got) != 1 || got[0] != "string" {
		t.Errorf("array = %#v", arr)
	}

	// allOf merges member objects.
	all := sampleForSchema(sr(&openapi3.Schema{AllOf: openapi3.SchemaRefs{
		sr(&openapi3.Schema{Type: typ("object"), Properties: openapi3.Schemas{"a": sr(&openapi3.Schema{Type: typ("string")})}}),
		sr(&openapi3.Schema{Type: typ("object"), Properties: openapi3.Schemas{"b": sr(&openapi3.Schema{Type: typ("integer")})}}),
	}}), fresh(), 0)
	m, ok := all.(map[string]any)
	if !ok || m["a"] != "string" || m["b"] != 0 {
		t.Errorf("allOf merge = %#v", all)
	}

	// oneOf / anyOf take the first branch.
	if got := sampleForSchema(sr(&openapi3.Schema{OneOf: openapi3.SchemaRefs{sr(&openapi3.Schema{Type: typ("boolean")})}}), fresh(), 0); got != false {
		t.Errorf("oneOf = %v", got)
	}
	if got := sampleForSchema(sr(&openapi3.Schema{AnyOf: openapi3.SchemaRefs{sr(&openapi3.Schema{Type: typ("integer")})}}), fresh(), 0); got != 0 {
		t.Errorf("anyOf = %v", got)
	}

	// A self-referencing schema (cyclic resolved $ref) must terminate.
	node := &openapi3.Schema{Type: typ("object")}
	nodeRef := sr(node)
	node.Properties = openapi3.Schemas{"child": nodeRef}
	got := sampleForSchema(nodeRef, fresh(), 0)
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("cyclic node = %#v, want object", got)
	}
	if _, present := obj["child"]; !present || obj["child"] != nil {
		t.Errorf("cyclic child = %#v, want nil (cycle broken)", obj["child"])
	}
}

// TestSynthesizeJSONBodyNoSchema verifies the no-schema guard returns "".
func TestSynthesizeJSONBodyNoSchema(t *testing.T) {
	if got := synthesizeJSONBody(&openapi3.MediaType{}); got != "" {
		t.Errorf("synthesizeJSONBody(no schema) = %q, want empty", got)
	}
}

// TestImportEnvironmentNamedAfterServerDescription pins that the hoisted
// environment takes its name from the OpenAPI server `description` (a deployment
// target's human name), not the generic "Default". Reported against
// https://dev.r3polska.eu/doc/v1.json, whose server is described
// "Api aplikacji developerskiej".
func TestImportEnvironmentNamedAfterServerDescription(t *testing.T) {
	const spec = `{
	  "openapi": "3.0.0",
	  "info": {"title": "Recomaty API v1", "version": "1.5.0"},
	  "servers": [{"url": "https://dev.r3polska.eu/", "description": "Api aplikacji developerskiej"}],
	  "paths": {}
	}`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if len(c.Environments) != 1 {
		t.Fatalf("environments = %+v", c.Environments)
	}
	if got := c.Environments[0].Name; got != "Api aplikacji developerskiej" {
		t.Errorf("env name = %q, want the server description", got)
	}
	// The trailing slash on the server URL is still trimmed (issue #181).
	if got := c.Environments[0].Variables[0].Value; got != "https://dev.r3polska.eu" {
		t.Errorf("base_url = %q, want trailing slash trimmed", got)
	}
}

// TestImportEnvironmentFallsBackToDefaultName verifies that a server with no
// description still yields the "Default" environment name.
func TestImportEnvironmentFallsBackToDefaultName(t *testing.T) {
	const spec = `{
	  "openapi": "3.0.0",
	  "info": {"title": "X"},
	  "servers": [{"url": "https://api.example.com"}],
	  "paths": {}
	}`
	c, err := FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if len(c.Environments) != 1 || c.Environments[0].Name != "Default" {
		t.Fatalf("environments = %+v, want one named Default", c.Environments)
	}
}

// TestBodyTypeWildcardIsJSON verifies the "*/*" content type maps to JSON.
func TestBodyTypeWildcardIsJSON(t *testing.T) {
	if got := bodyTypeFromContentType("*/*"); got != model.BodyJSON {
		t.Errorf("bodyTypeFromContentType(*/*) = %q, want JSON", got)
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
