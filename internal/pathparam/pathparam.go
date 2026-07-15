package pathparam

import "strings"

// Path parameters are single-brace {name} placeholders embedded in a request
// URL's path, e.g. "{{base_url}}/drs/bag/{bagId}". They are distinct from
// vars-package {{variable}} templates (double brace): a {{...}} template is
// resolved from the collection / environment scopes, while a {name} path
// parameter is filled from the request's own Path tab. Scanning here always
// steps over {{...}} spans so the inner name of a template like {{base_url}}
// is never mistaken for a path parameter.

// Names returns the distinct path-parameter names in s — the {name} tokens
// that are NOT part of a {{variable}} template — in first-seen order.
func Names(s string) []string {
	var out []string
	seen := map[string]bool{}
	walk(s, func(name string) string {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		return "{" + name + "}"
	})
	return out
}

// Apply replaces each {name} token in s (skipping {{variable}} templates) with
// the value lookup returns for that name. When lookup reports ok=false the token
// is left intact, so an unfilled path parameter stays visible as {name} rather
// than collapsing to an empty segment. Inserted values are never re-scanned, so
// a value that itself contains braces cannot spawn further substitution.
func Apply(s string, lookup func(name string) (value string, ok bool)) string {
	return walk(s, func(name string) string {
		if v, ok := lookup(name); ok {
			return v
		}
		return "{" + name + "}"
	})
}

// isNameByte reports whether b may appear inside a path-parameter name. A name
// spans a single path segment, so segment / URL delimiters and whitespace end
// it; braces end it too (and guard against matching into a {{template}}).
func isNameByte(b byte) bool {
	switch b {
	case '{', '}', '/', '?', '#', ' ', '\t', '\n', '\r':
		return false
	default:
		return true
	}
}

// walk scans s left to right, invoking repl for each single-brace {name} token
// and copying everything else verbatim. A {{...}} template is copied whole (its
// closing }} included) so its inner text is never treated as a token. A lone {
// that does not open a valid {name} token is emitted as-is. repl's return value
// replaces the token and is not re-scanned.
func walk(s string, repl func(name string) string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			// {{ ... }} template: copy through the closing }} (or to end if none).
			if end := strings.Index(s[i:], "}}"); end >= 0 {
				b.WriteString(s[i : i+end+2])
				i += end + 2
			} else {
				b.WriteString(s[i:])
				i = len(s)
			}
			continue
		}
		// Single '{': read a name up to the closing '}'.
		j := i + 1
		for j < len(s) && isNameByte(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '}' && j > i+1 {
			b.WriteString(repl(s[i+1 : j]))
			i = j + 1
			continue
		}
		b.WriteByte('{')
		i++
	}
	return b.String()
}
