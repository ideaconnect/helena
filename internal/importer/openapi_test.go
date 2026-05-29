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
