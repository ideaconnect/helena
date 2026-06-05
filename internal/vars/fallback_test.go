package vars

import "testing"

// TestResolverFallback verifies the dynamic fallback resolves names no scope
// has, that scopes still win over the fallback, and that a resolver with no
// fallback behaves exactly as before.
func TestResolverFallback(t *testing.T) {
	r := New(map[string]string{"base": "from-scope"}).WithFallback(func(name string) (string, bool) {
		switch name {
		case "base":
			return "from-fallback", true // must be shadowed by the scope
		case "chain.login.response.json.token":
			return "abc123", true
		}
		return "", false
	})

	if got, miss := r.Resolve("{{base}}/{{chain.login.response.json.token}}"); got != "from-scope/abc123" || len(miss) != 0 {
		t.Errorf("Resolve = %q missing=%v, want from-scope/abc123 + none", got, miss)
	}
	// A name neither a scope nor the fallback resolves stays unresolved.
	if got, miss := r.Resolve("{{nope}}"); got != "{{nope}}" || len(miss) != 1 || miss[0] != "nope" {
		t.Errorf("Resolve(nope) = %q missing=%v", got, miss)
	}

	// No fallback set → unchanged behavior.
	plain := New(map[string]string{"x": "y"})
	if got, _ := plain.Resolve("{{x}}-{{z}}"); got != "y-{{z}}" {
		t.Errorf("no-fallback Resolve = %q, want y-{{z}}", got)
	}
}
