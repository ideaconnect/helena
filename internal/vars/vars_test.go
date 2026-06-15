package vars

import (
	"reflect"
	"strconv"
	"testing"
)

// TestResolveSimple verifies that a single {{name}} reference is replaced by its scope value.
func TestResolveSimple(t *testing.T) {
	r := New(map[string]string{"host": "example.com"})
	got, missing := r.Resolve("https://{{host}}/api")
	if got != "https://example.com/api" {
		t.Errorf("got %q", got)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

// TestResolveWhitespace verifies that surrounding whitespace inside {{ name }} is ignored.
func TestResolveWhitespace(t *testing.T) {
	r := New(map[string]string{"host": "example.com"})
	if got, _ := r.Resolve("{{ host }}"); got != "example.com" {
		t.Errorf("got %q", got)
	}
}

// TestResolveMultipleAndMissing verifies that known refs are substituted, unknown ones survive and are reported once.
func TestResolveMultipleAndMissing(t *testing.T) {
	r := New(map[string]string{"host": "x.com"})
	got, missing := r.Resolve("{{host}}/{{version}}/{{host}}")
	if got != "x.com/{{version}}/x.com" {
		t.Errorf("got %q", got)
	}
	if !reflect.DeepEqual(missing, []string{"version"}) {
		t.Errorf("missing = %v, want [version]", missing)
	}
}

// TestResolvePrecedence verifies that later scopes override earlier ones for both Lookup and Resolve.
func TestResolvePrecedence(t *testing.T) {
	r := New(map[string]string{"x": "low"}, map[string]string{"x": "high"})
	if v, ok := r.Lookup("x"); !ok || v != "high" {
		t.Errorf("Lookup = %q ok=%v, want high", v, ok)
	}
	if got, _ := r.Resolve("{{x}}"); got != "high" {
		t.Errorf("got %q, want high", got)
	}
}

// TestResolveChained verifies that variables whose values reference other variables are resolved to a fixed point.
func TestResolveChained(t *testing.T) {
	r := New(map[string]string{
		"url":   "{{proto}}://{{host}}",
		"proto": "https",
		"host":  "x.com",
	})
	got, missing := r.Resolve("{{url}}/p")
	if got != "https://x.com/p" {
		t.Errorf("got %q", got)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

// TestResolveNoVars verifies that input without templates is returned unchanged with no missing names.
func TestResolveNoVars(t *testing.T) {
	r := New(nil)
	got, missing := r.Resolve("plain text")
	if got != "plain text" || len(missing) != 0 {
		t.Errorf("got %q missing %v", got, missing)
	}
}

// TestResolveCycleTerminates verifies that a cyclic reference (a->b->a) halts without looping and is reported as unresolved.
func TestResolveCycleTerminates(t *testing.T) {
	r := New(map[string]string{"a": "{{b}}", "b": "{{a}}"})
	got, missing := r.Resolve("{{a}}")
	if got != "{{a}}" && got != "{{b}}" {
		t.Errorf("unexpected got %q", got)
	}
	if len(missing) == 0 {
		t.Errorf("expected unresolved names for a cycle, got none")
	}
}

// TestResolveFallbackValueIsFrozen is the injection regression: a dynamic
// fallback value (e.g. response/chain data) that itself contains {{secret}}
// must be substituted verbatim, never re-expanded against the user's scopes.
func TestResolveFallbackValueIsFrozen(t *testing.T) {
	r := New(map[string]string{"secret": "TOPSECRET"}).
		WithFallback(func(name string) (string, bool) {
			if name == "chain.login.response.body" {
				return "leak={{secret}}", true // attacker-controlled payload
			}
			return "", false
		})
	got, missing := r.Resolve("x={{chain.login.response.body}}")
	if got != "x=leak={{secret}}" {
		t.Errorf("fallback value was re-expanded (injection): got %q, want %q", got, "x=leak={{secret}}")
	}
	if len(missing) != 0 {
		// The frozen {{secret}} is not a top-level template, so it is not "missing".
		t.Errorf("missing = %v, want none", missing)
	}
}

// TestResolveScopeCompositionRecurses verifies user-authored scope variables
// still compose recursively (the documented feature TestResolveChained pins),
// including beyond the old 10-pass cap, resolving fully rather than being
// mis-reported as missing.
func TestResolveScopeCompositionRecurses(t *testing.T) {
	const depth = 15
	scope := map[string]string{"v" + strconv.Itoa(depth): "DONE"}
	for i := 0; i < depth; i++ {
		scope["v"+strconv.Itoa(i)] = "{{v" + strconv.Itoa(i+1) + "}}"
	}
	r := New(scope)
	got, missing := r.Resolve("{{v0}}")
	if got != "DONE" {
		t.Errorf("deep chain not fully resolved: got %q, want DONE", got)
	}
	if len(missing) != 0 {
		t.Errorf("resolvable deep chain mis-reported as missing: %v", missing)
	}
}

// TestResolveEmptyTemplate verifies an empty {{}} is left untouched and not reported.
func TestResolveEmptyTemplate(t *testing.T) {
	got, missing := New(nil).Resolve("a {{}} b")
	if got != "a {{}} b" || len(missing) != 0 {
		t.Errorf("got %q missing %v", got, missing)
	}
}
