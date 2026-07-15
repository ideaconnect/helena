package pathparam

import (
	"reflect"
	"testing"
)

// TestNames verifies token extraction skips {{templates}}, dedupes, and keeps
// first-seen order across the syntaxes that share a URL.
func TestNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"none", "https://api.example.com/v1/ping", nil},
		{"single", "{{base_url}}/drs/bag/{bagId}", []string{"bagId"}},
		{"multiple", "/users/{userId}/orders/{orderId}", []string{"userId", "orderId"}},
		{"dupe", "/a/{id}/b/{id}", []string{"id"}},
		{"skip-template", "{{base_url}}/{{version}}/x/{id}", []string{"id"}},
		{"template-only", "{{base_url}}/{{path}}", nil},
		{"hyphen-dot", "/x/{user-id}/{ns.key}", []string{"user-id", "ns.key"}},
		{"query-token-untouched", "/x/{id}?q={notpath}", []string{"id", "notpath"}},
		{"lone-brace", "/a/{/b", nil},
		{"empty-braces", "/a/{}/b", nil},
		// A malformed, unterminated {{ swallows the rest rather than exposing the
		// template's inner name (base_url) as a spurious path parameter — the
		// safer failure mode for mid-typing input.
		{"unterminated-template", "{{base_url}/x/{id}", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Names(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Names(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestApply verifies token replacement fills known names, leaves unknown/unfilled
// tokens intact, steps over {{templates}}, and never re-scans inserted values.
func TestApply(t *testing.T) {
	lookup := map[string]string{"bagId": "42", "id": "a/b", "brace": "{id}"}
	repl := func(name string) (string, bool) {
		v, ok := lookup[name]
		return v, ok
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"fill", "{{base_url}}/drs/bag/{bagId}", "{{base_url}}/drs/bag/42"},
		{"unknown-left", "/x/{missing}", "/x/{missing}"},
		{"skip-template", "{{bagId}}/{bagId}", "{{bagId}}/42"},
		{"raw-slash-value", "/x/{id}", "/x/a/b"},
		{"no-rescan", "/x/{brace}/{id}", "/x/{id}/a/b"},
		{"mixed", "/{bagId}/{missing}/{id}", "/42/{missing}/a/b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Apply(c.in, repl); got != c.want {
				t.Errorf("Apply(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestApplyLeavesTokenWhenNotOk verifies a lookup that reports ok=false leaves
// the token untouched even when it returns a non-empty value.
func TestApplyLeavesTokenWhenNotOk(t *testing.T) {
	got := Apply("/x/{id}", func(string) (string, bool) { return "ignored", false })
	if got != "/x/{id}" {
		t.Errorf("Apply = %q, want the token left intact", got)
	}
}
