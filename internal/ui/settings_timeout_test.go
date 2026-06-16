package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestSanitizeMaxResponseMiB verifies the response-cap field parses MiB into
// bytes and rejects blank/non-numeric/non-positive entries to a safe positive
// fallback (#111).
func TestSanitizeMaxResponseMiB(t *testing.T) {
	const def = model.DefaultMaxResponseBytes
	cases := []struct {
		text     string
		fallback int64
		want     int64
		valid    bool
	}{
		{"100", def, 100 << 20, true}, // valid MiB -> bytes
		{" 5 ", def, 5 << 20, true},   // trimmed
		{"", def, def, false},         // blank -> fallback
		{"abc", def, def, false},      // non-numeric -> fallback
		{"0", def, def, false},        // zero rejected -> fallback
		{"-3", def, def, false},       // negative rejected -> fallback
		{"x", 0, def, false},          // bad fallback floored to default
		{"x", -1, def, false},         // negative fallback floored to default
	}
	for _, c := range cases {
		got, valid := sanitizeMaxResponseMiB(c.text, c.fallback)
		if got != c.want || valid != c.valid {
			t.Errorf("sanitizeMaxResponseMiB(%q,%d) = (%d,%v), want (%d,%v)", c.text, c.fallback, got, valid, c.want, c.valid)
		}
		if got <= 0 {
			t.Errorf("sanitizeMaxResponseMiB(%q,%d) returned non-positive %d", c.text, c.fallback, got)
		}
	}
}

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
