package vars

import (
	"regexp"
	"strings"
)

// varRe matches a {{ name }} template, capturing the (untrimmed) name.
var varRe = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)

// maxPasses caps fixed-point resolution so cyclic references cannot loop forever.
const maxPasses = 10

// Resolver substitutes {{name}} templates using a set of named scopes.
// Scopes are supplied lowest-precedence first; later scopes override earlier
// ones — e.g. New(collectionVars, envVars, dotEnvVars).
type Resolver struct {
	scopes   []map[string]string
	fallback func(name string) (string, bool)
}

// New builds a Resolver from scopes ordered low to high precedence.
func New(scopes ...map[string]string) *Resolver {
	return &Resolver{scopes: scopes}
}

// WithFallback attaches a dynamic lookup consulted for any name no scope
// resolves. It lets callers inject namespaced values — e.g.
// {{chain.<alias>.response.json.token}} backed by chain results — without the
// resolver knowing their source. Returns r for chaining. Passing nil clears it.
func (r *Resolver) WithFallback(fn func(name string) (string, bool)) *Resolver {
	r.fallback = fn
	return r
}

// Lookup returns the highest-precedence value for name, falling back to the
// dynamic lookup (if any) when no scope has it.
func (r *Resolver) Lookup(name string) (string, bool) {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if v, ok := r.scopes[i][name]; ok {
			return v, true
		}
	}
	if r.fallback != nil {
		return r.fallback(name)
	}
	return "", false
}

// Resolve substitutes every {{name}} in s with its value, repeating until a
// fixed point so chained variables resolve. It returns the result and the names
// that remain unresolved (missing or cyclic), deduped in first-seen order.
func (r *Resolver) Resolve(s string) (string, []string) {
	cur := s
	for i := 0; i < maxPasses; i++ {
		next := r.substituteOnce(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return cur, unresolvedNames(cur)
}

func (r *Resolver) substituteOnce(s string) string {
	return varRe.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimSpace(varRe.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}
		if v, ok := r.Lookup(name); ok {
			return v
		}
		return match
	})
}

func unresolvedNames(s string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, m := range varRe.FindAllStringSubmatch(s, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
