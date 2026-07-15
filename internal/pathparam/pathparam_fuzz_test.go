package pathparam

import (
	"strings"
	"testing"
)

// FuzzWalk feeds arbitrary strings through Names and Apply looking for crashes
// or non-termination, and asserts the core invariants hold for every input:
// walk always returns, an identity Apply is a no-op, no {{template}} inner name
// is ever exposed as a path parameter, and every name Names reports round-trips
// through Apply's replacement.
func FuzzWalk(f *testing.F) {
	for _, s := range []string{
		"",
		"plain",
		"{a}",
		"{{a}}",
		"{{a}}/{b}",
		"{a}}",
		"{{a}",
		"{}",
		"{{}}",
		"a{b}c{d}e",
		"/x/{a/b}",
		"{a}{b}",
		"{{a}}{b}{{c}}{d}",
		"\x00{\x00}",
		"{" + strings.Repeat("x", 100) + "}",
		"?tag={x}#frag{y}",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// Identity Apply must be a no-op: replacing every {name} with the same
		// {name} reproduces the input exactly.
		if got := Apply(s, func(n string) (string, bool) { return "{" + n + "}", true }); got != s {
			t.Fatalf("identity Apply changed input:\n in  %q\n out %q", s, got)
		}

		names := Names(s)
		seen := map[string]bool{}
		for _, n := range names {
			if n == "" {
				t.Fatalf("Names returned an empty token for %q", s)
			}
			if seen[n] {
				t.Fatalf("Names returned a duplicate %q for %q", n, s)
			}
			seen[n] = true
			// A reported name must never contain a brace — that would mean a
			// {{template}} span leaked into a path-parameter token.
			if strings.ContainsAny(n, "{}") {
				t.Fatalf("Names token %q contains a brace (template leak) for %q", n, s)
			}
			// Every reported name is fillable by Apply.
			out := Apply(s, func(x string) (string, bool) {
				if x == n {
					return "<F>", true
				}
				return "", false
			})
			if out == s && strings.Contains(s, "{"+n+"}") {
				t.Fatalf("Apply did not replace reported name %q in %q", n, s)
			}
		}
	})
}
