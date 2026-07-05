// Package importer converts external API descriptions (OpenAPI 3, Swagger 2,
// later WSDL) into Helena collections.
package importer

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// FromOpenAPI parses an OpenAPI 3 or Swagger 2 spec (JSON or YAML bytes) and
// returns the resulting Helena collection. The spec version is auto-detected.
func FromOpenAPI(data []byte) (col model.Collection, err error) {
	// kin-openapi's Swagger-2 conversion and OAS3 traversal can panic on some
	// malformed specs (e.g. a null server or operation object). An importer must
	// return an error, not crash the app — a hostile URL-served spec reaches this
	// path off the UI goroutine with no recover above it (unlike the file/cURL
	// paths, which run under the UI panic guard). Turn any panic into an error.
	defer func() {
		if r := recover(); r != nil {
			col = model.Collection{}
			err = fmt.Errorf("malformed spec: %v", r)
		}
	}()

	asJSON, err := toJSON(data)
	if err != nil {
		return model.Collection{}, fmt.Errorf("parse spec: %w", err)
	}

	var probe map[string]any
	if err := json.Unmarshal(asJSON, &probe); err != nil {
		return model.Collection{}, fmt.Errorf("decode spec: %w", err)
	}

	var doc *openapi3.T
	switch {
	case probe["swagger"] != nil:
		var v2 openapi2.T
		if err := json.Unmarshal(asJSON, &v2); err != nil {
			return model.Collection{}, fmt.Errorf("swagger 2 parse: %w", err)
		}
		doc, err = openapi2conv.ToV3(&v2)
		if err != nil {
			return model.Collection{}, fmt.Errorf("swagger 2 -> oas3: %w", err)
		}
	case probe["openapi"] != nil:
		loader := openapi3.NewLoader()
		doc, err = loader.LoadFromData(asJSON)
		if err != nil {
			return model.Collection{}, fmt.Errorf("oas3 load: %w", err)
		}
	default:
		return model.Collection{}, fmt.Errorf("not an OpenAPI/Swagger document (no openapi or swagger key)")
	}

	return convertOAS3(doc), nil
}

// toJSON returns data as JSON. JSON inputs are returned unchanged; YAML inputs
// are parsed and re-marshaled as JSON.
func toJSON(data []byte) ([]byte, error) {
	if json.Valid(data) {
		return data, nil
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	v = normalizeYAML(v)
	return json.Marshal(v)
}

// normalizeYAML walks the decoded YAML tree and converts map[interface{}]
// interface{} (which yaml.v3 still produces for unkeyed nested maps) into
// map[string]any so json.Marshal can encode it.
func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, vv := range x {
			m[fmt.Sprint(k)] = normalizeYAML(vv)
		}
		return m
	case map[string]any:
		for k, vv := range x {
			x[k] = normalizeYAML(vv)
		}
		return x
	case []any:
		for i, vv := range x {
			x[i] = normalizeYAML(vv)
		}
		return x
	}
	return v
}

func convertOAS3(doc *openapi3.T) model.Collection {
	// doc.Info is optional in practice — a spec can parse with no `info` block,
	// so guard the deref (a nil Info would otherwise panic and crash the app,
	// since the import callbacks have no recover()).
	name := "Imported API"
	if doc.Info != nil {
		name = stringOr(doc.Info.Title, name)
	}
	c := model.Collection{
		ID:   model.NewID(),
		Name: name,
	}

	baseURL := ""
	for _, srv := range doc.Servers {
		// A spec may carry `servers: [null]`; the loader stores a nil *Server, so
		// guard the deref and take the first server that actually names a URL.
		if srv != nil && srv.URL != "" {
			baseURL = srv.URL
			break
		}
	}
	if baseURL != "" {
		c.Environments = append(c.Environments, model.Environment{
			ID:   model.NewID(),
			Name: "Default",
			Variables: []model.Variable{{
				Enabled: true,
				Key:     "base_url",
				Value:   baseURL,
			}},
		})
	}

	folders := map[string]*model.Folder{}
	var folderOrder []string

	if doc.Paths != nil {
		for _, path := range sortedKeys(doc.Paths.Map()) {
			pathItem := doc.Paths.Value(path)
			for _, method := range sortedKeys(pathItem.Operations()) {
				op := pathItem.Operations()[method]
				req := buildRequest(method, path, baseURL != "", op, pathItem)

				tag := ""
				if len(op.Tags) > 0 {
					tag = op.Tags[0]
				}
				if tag == "" {
					c.Requests = append(c.Requests, req)
					continue
				}
				f, ok := folders[tag]
				if !ok {
					f = &model.Folder{ID: model.NewID(), Name: tag}
					folders[tag] = f
					folderOrder = append(folderOrder, tag)
				}
				f.Requests = append(f.Requests, req)
			}
		}
	}
	for _, tag := range folderOrder {
		c.Folders = append(c.Folders, *folders[tag])
	}
	return c
}

func buildRequest(method, path string, hasBaseVar bool, op *openapi3.Operation, pathItem *openapi3.PathItem) model.Request {
	r := model.Request{
		ID:     model.NewID(),
		Name:   requestName(op, method, path),
		Method: model.Method(method),
		Body:   model.Body{Type: model.BodyNone},
	}
	if hasBaseVar {
		r.URL = "{{base_url}}" + path
	} else {
		r.URL = path
	}

	// Combine path-item params and operation params; deduplicate by (in,name).
	seen := map[string]bool{}
	for _, ref := range append(append([]*openapi3.ParameterRef{}, pathItem.Parameters...), op.Parameters...) {
		if ref == nil || ref.Value == nil {
			continue
		}
		p := ref.Value
		key := p.In + ":" + p.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		kv := model.KeyValue{
			Enabled:     p.Required, // start required params enabled; optional disabled
			Key:         p.Name,
			Value:       defaultParamValue(p),
			Description: p.Description,
		}
		switch p.In {
		case "query":
			r.Params = append(r.Params, kv)
		case "header":
			r.Headers = append(r.Headers, kv)
			// path params: left embedded in the URL as {name}
		}
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, ct := range sortedKeys(op.RequestBody.Value.Content) {
			mt := op.RequestBody.Value.Content[ct]
			if mt == nil {
				continue
			}
			r.Body = model.Body{
				Type:    bodyTypeFromContentType(ct),
				Content: extractExample(mt),
			}
			break
		}
	}
	return r
}

func requestName(op *openapi3.Operation, method, path string) string {
	if op.Summary != "" {
		return op.Summary
	}
	if op.OperationID != "" {
		return op.OperationID
	}
	return method + " " + path
}

func defaultParamValue(p *openapi3.Parameter) string {
	if p.Example != nil {
		return formatExample(p.Example)
	}
	if p.Schema != nil && p.Schema.Value != nil && p.Schema.Value.Default != nil {
		return formatExample(p.Schema.Value.Default)
	}
	return ""
}

func bodyTypeFromContentType(ct string) model.BodyType {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "json"):
		return model.BodyJSON
	case strings.Contains(ct, "xml"):
		return model.BodyXML
	case strings.Contains(ct, "form-urlencoded"):
		return model.BodyForm
	case strings.Contains(ct, "multipart"):
		return model.BodyMultipart
	case strings.HasPrefix(ct, "text/"):
		return model.BodyText
	default:
		return model.BodyText
	}
}

func extractExample(mt *openapi3.MediaType) string {
	if mt.Example != nil {
		return formatExample(mt.Example)
	}
	for _, ex := range mt.Examples {
		if ex != nil && ex.Value != nil && ex.Value.Value != nil {
			return formatExample(ex.Value.Value)
		}
	}
	if mt.Schema != nil && mt.Schema.Value != nil && mt.Schema.Value.Example != nil {
		return formatExample(mt.Schema.Value.Example)
	}
	return ""
}

func formatExample(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func stringOr(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Insertion-ordered would be nicer, but kin-openapi's map is unordered; a
	// stable sorted order keeps imports deterministic. slices.Sort is O(n log n)
	// — a spec with tens of thousands of paths must not go quadratic.
	slices.Sort(keys)
	return keys
}
