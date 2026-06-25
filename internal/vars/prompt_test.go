package vars

import (
	"reflect"
	"testing"
)

// TestPromptVarsExtracts verifies prompt keys are found across texts, distinct
// and first-seen ordered, and that non-prompt vars and bare "{{?}}" are ignored.
func TestPromptVarsExtracts(t *testing.T) {
	got := PromptVars(
		"https://{{host}}/{{?Env}}/users?token={{?Token}}",
		"Authorization: Bearer {{?Token}}", // duplicate Token
		"{{normal}} {{? }}",                // non-prompt + empty prompt ignored
	)
	want := []string{"?Env", "?Token"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PromptVars = %v, want %v", got, want)
	}
}

// TestPromptVarsNone verifies text with no prompt vars yields nil.
func TestPromptVarsNone(t *testing.T) {
	if got := PromptVars("{{host}}/path", "no vars here"); got != nil {
		t.Errorf("PromptVars = %v, want nil", got)
	}
}

// TestPromptLabel verifies the '?' marker is stripped for display.
func TestPromptLabel(t *testing.T) {
	if got := PromptLabel("?API Key"); got != "API Key" {
		t.Errorf("PromptLabel = %q, want %q", got, "API Key")
	}
}

// TestPromptVarResolvesViaScope verifies that injecting a scope under the
// prompt key resolves the {{?Name}} reference end-to-end (no special resolver
// support needed — the key is the captured token).
func TestPromptVarResolvesViaScope(t *testing.T) {
	keys := PromptVars("https://x/{{?Env}}")
	if len(keys) != 1 {
		t.Fatalf("keys = %v, want one", keys)
	}
	r := New(map[string]string{keys[0]: "staging"})
	out, missing := r.Resolve("https://x/{{?Env}}")
	if len(missing) != 0 || out != "https://x/staging" {
		t.Errorf("resolve = %q missing %v, want https://x/staging", out, missing)
	}
}
