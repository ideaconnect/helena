package session

import (
	"strings"
	"testing"
)

// FuzzSplitChainPath verifies the chain-ref path filter survives any
// input string with three guarantees:
//
//   - never panics
//   - the returned slice never contains an empty segment, a literal
//     "/" inside a segment, or a "."/".." traversal token (those
//     cases must surface as nil per the Phase 7.4 hardening)
//   - is idempotent: splitting the joined-back form returns the same
//     parts (no information loss)
//
// This is the gateway for every chain-ref lookup, including refs
// from imported collections — adversarial inputs must not be able to
// escape the active collection's name space.
func FuzzSplitChainPath(f *testing.F) {
	for _, s := range []string{
		"",
		"a",
		"a/b",
		"/leading",
		"trailing/",
		"a//b",
		" a / b ",
		"a/./b",
		"../escape",
		".",
		"..",
		"./.",
		"a/b/c/d/e/f/g/h",
		"\x00",
		strings.Repeat("a/", 100) + "leaf",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		parts := splitChainPath(ref)
		// nil is a legitimate output (rejected). Anything else must
		// satisfy the invariants.
		for _, p := range parts {
			if p == "" {
				t.Errorf("empty segment in %v from %q", parts, ref)
			}
			if strings.Contains(p, "/") {
				t.Errorf("segment %q contains separator (from %q)", p, ref)
			}
			if p == "." || p == ".." {
				t.Errorf("dot/dotdot segment %q survived from %q", p, ref)
			}
			if strings.TrimSpace(p) != p {
				t.Errorf("segment %q not whitespace-trimmed (from %q)", p, ref)
			}
		}
		// Round-trip via the canonical join.
		if len(parts) > 0 {
			joined := strings.Join(parts, "/")
			parts2 := splitChainPath(joined)
			if !equalStrings(parts, parts2) {
				t.Errorf("not idempotent: %v then %v (input %q)", parts, parts2, ref)
			}
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
