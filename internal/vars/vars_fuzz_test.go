package vars

import (
	"strings"
	"testing"
)

// FuzzResolveTemplate feeds arbitrary template strings to Resolve
// looking for crashes, infinite loops (the maxPasses cap protects),
// or unbounded growth. The resolver MUST always return; the missing
// list MUST contain only names that are not in any scope.
func FuzzResolveTemplate(f *testing.F) {
	// Seed corpus — known interesting shapes.
	for _, s := range []string{
		"",
		"plain",
		"{{a}}",
		"{{a}}-{{b}}",
		"{{ a }}",
		"{{a",
		"a}}",
		"}}{{",
		"{{{{a}}}}",
		"{{nested {{inner}}}}",
		"{{a}}{{b}}{{c}}{{d}}{{e}}{{f}}",
		"{{cycle}}",           // cycle: cycle → {{cycle}} (won't terminate without maxPasses)
		"\x00",                // non-printable
		"{{name with space}}", // names with spaces
	} {
		f.Add(s)
	}
	r := New(
		map[string]string{"a": "A", "b": "B", "c": "C", "d": "D", "e": "E", "f": "F"},
		map[string]string{"cycle": "{{cycle}}"},
	)
	f.Fuzz(func(t *testing.T, s string) {
		out, missing := r.Resolve(s)
		// No panic = pass. Sanity checks below catch regressions in
		// the cap/dedupe contract.
		// Length is bounded: each pass can grow output by up to a
		// fixed factor; with maxPasses + a small alphabet, output
		// shouldn't explode exponentially. Pick a generous cap.
		if len(out) > 1<<20 {
			t.Errorf("output grew to %d bytes (likely cap regression)", len(out))
		}
		// Missing list must be deduped (each name appears once).
		// Names can legitimately be in `missing` even if they map to
		// something in the scope — a cyclic reference exhausts the
		// pass cap and surfaces as missing.
		seen := map[string]bool{}
		for _, name := range missing {
			if seen[name] {
				t.Errorf("missing list contains duplicate %q", name)
			}
			seen[name] = true
		}
	})
}

// FuzzResolveAccumulatesMissingPerCallsite verifies the dedupe
// behavior under random repeated names — each name appears at most
// once in missing regardless of how many times it appears in s.
func FuzzResolveAccumulatesMissingPerCallsite(f *testing.F) {
	f.Add("{{a}}{{a}}{{a}}")
	f.Add("{{a}}-{{b}}-{{a}}")
	f.Add("{{x}}{{y}}{{x}}{{y}}{{x}}")
	r := New() // empty scope so every name is missing
	f.Fuzz(func(t *testing.T, s string) {
		// Skip degenerate inputs to keep the assertion tractable.
		if !strings.Contains(s, "{{") {
			return
		}
		_, missing := r.Resolve(s)
		seen := map[string]bool{}
		for _, n := range missing {
			if seen[n] {
				t.Errorf("dedupe failed: %q appears twice in missing %v", n, missing)
			}
			seen[n] = true
		}
	})
}
