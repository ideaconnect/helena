package ui

import "testing"

// TestSanitizeTimeoutSeconds verifies an invalid/blank/negative/zero timeout
// never silently becomes 0 (== unlimited); it falls back to a positive value
// and is flagged invalid (regression for #97).
func TestSanitizeTimeoutSeconds(t *testing.T) {
	cases := []struct {
		text     string
		fallback int
		want     int
		valid    bool
	}{
		{"45", 30, 45, true},    // valid positive
		{"  60 ", 30, 60, true}, // trimmed
		{"", 30, 30, false},     // blank -> fallback, not 0
		{"abc", 30, 30, false},  // non-numeric -> fallback
		{"-5", 30, 30, false},   // negative -> fallback
		{"0", 30, 30, false},    // zero (unlimited) rejected -> fallback
		{"0", 0, 30, false},     // bad fallback floored to default 30
		{"x", -1, 30, false},    // negative fallback floored to default 30
	}
	for _, c := range cases {
		got, valid := sanitizeTimeoutSeconds(c.text, c.fallback)
		if got != c.want || valid != c.valid {
			t.Errorf("sanitizeTimeoutSeconds(%q,%d) = (%d,%v), want (%d,%v)", c.text, c.fallback, got, valid, c.want, c.valid)
		}
		if got <= 0 {
			t.Errorf("sanitizeTimeoutSeconds(%q,%d) returned non-positive %d (would be unlimited)", c.text, c.fallback, got)
		}
	}
}
