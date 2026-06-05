package chain

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// VarLookup returns a [vars.Resolver] fallback (a func(name string) (string,
// bool)) that resolves {{chain.<alias>.<path>}} template names against the
// chain results in views. Names without the "chain." prefix return ok=false so
// the resolver keeps treating them as unresolved.
//
// The path mirrors the script-side chain.<alias> surface so users have one
// mental model:
//
//	chain.<alias>.response.json.<dotted.path>   // array elements by index: items.0.id
//	chain.<alias>.response.headers.<Header-Name>
//	chain.<alias>.response.status | .statusText | .body | .text
//	chain.<alias>.request.method | .url | .body
//
// Only scalar leaves resolve (string / number / bool / null → empty); a path
// that lands on a JSON object or array, an unknown alias, or a missing path
// returns ok=false, so httpclient.Build reports it as an unresolved variable.
// XML bodies are not navigable from templates — use a script for those.
func VarLookup(views map[string]View) func(string) (string, bool) {
	return func(name string) (string, bool) {
		rest, ok := strings.CutPrefix(name, "chain.")
		if !ok {
			return "", false
		}
		alias, path, _ := strings.Cut(rest, ".")
		v, ok := views[alias]
		if !ok {
			return "", false
		}
		return lookupInView(v, path)
	}
}

// lookupInView dispatches on the first path segment (request vs response).
func lookupInView(v View, path string) (string, bool) {
	section, rest, _ := strings.Cut(path, ".")
	switch section {
	case "request":
		switch rest {
		case "method":
			return v.Request.Method, true
		case "url":
			return v.Request.URL, true
		case "body":
			return string(v.Request.Body), true
		}
	case "response":
		return lookupInResponse(v.Response, rest)
	}
	return "", false
}

// lookupInResponse resolves the response-side paths.
func lookupInResponse(r ResponseView, path string) (string, bool) {
	field, rest, _ := strings.Cut(path, ".")
	switch field {
	case "status":
		return strconv.Itoa(r.StatusCode), true
	case "statusText":
		return r.Status, true
	case "body", "text":
		return string(r.Body), true
	case "headers":
		if rest == "" {
			return "", false
		}
		if vals := r.Headers.Values(rest); len(vals) > 0 {
			return vals[0], true
		}
	case "json":
		return lookupJSONPath(r.Body, rest)
	}
	return "", false
}

// lookupJSONPath decodes body as JSON and walks the dotted path to a scalar.
// An empty path returns the whole document only when it is itself a scalar.
func lookupJSONPath(body []byte, path string) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // keep numbers as their exact source text
	var cur any
	if err := dec.Decode(&cur); err != nil {
		return "", false
	}
	if path != "" {
		for _, seg := range strings.Split(path, ".") {
			switch node := cur.(type) {
			case map[string]any:
				next, ok := node[seg]
				if !ok {
					return "", false
				}
				cur = next
			case []any:
				idx, err := strconv.Atoi(seg)
				if err != nil || idx < 0 || idx >= len(node) {
					return "", false
				}
				cur = node[idx]
			default:
				return "", false // cannot descend into a scalar
			}
		}
	}
	return scalarString(cur)
}

// scalarString renders a JSON scalar as a string. Objects and arrays are not
// scalars and return ok=false; an explicit null renders as the empty string.
func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		return strconv.FormatBool(x), true
	case nil:
		return "", true
	default:
		return "", false
	}
}
