package vars

import (
	"reflect"
	"testing"
)

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

func TestResolveWhitespace(t *testing.T) {
	r := New(map[string]string{"host": "example.com"})
	if got, _ := r.Resolve("{{ host }}"); got != "example.com" {
		t.Errorf("got %q", got)
	}
}

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

func TestResolvePrecedence(t *testing.T) {
	r := New(map[string]string{"x": "low"}, map[string]string{"x": "high"})
	if v, ok := r.Lookup("x"); !ok || v != "high" {
		t.Errorf("Lookup = %q ok=%v, want high", v, ok)
	}
	if got, _ := r.Resolve("{{x}}"); got != "high" {
		t.Errorf("got %q, want high", got)
	}
}

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

func TestResolveNoVars(t *testing.T) {
	r := New(nil)
	got, missing := r.Resolve("plain text")
	if got != "plain text" || len(missing) != 0 {
		t.Errorf("got %q missing %v", got, missing)
	}
}

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
