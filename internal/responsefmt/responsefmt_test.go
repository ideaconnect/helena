package responsefmt

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPrettyJSON verifies that PrettyJSON indents valid JSON with two spaces and errors on invalid input.
func TestPrettyJSON(t *testing.T) {
	got, err := PrettyJSON([]byte(`{"a":1,"b":[2,3]}`))
	if err != nil {
		t.Fatalf("PrettyJSON: %v", err)
	}
	if !strings.Contains(got, "\n  \"a\": 1") || !strings.Contains(got, "[\n    2,") {
		t.Errorf("PrettyJSON did not indent as expected:\n%s", got)
	}
	if _, err := PrettyJSON([]byte("not json")); err == nil {
		t.Errorf("PrettyJSON should error on invalid input")
	}
}

// TestPrettyXML verifies that PrettyXML indents well-formed XML and errors on mismatched tags.
func TestPrettyXML(t *testing.T) {
	got, err := PrettyXML([]byte(`<root><a>x</a><b>y</b></root>`))
	if err != nil {
		t.Fatalf("PrettyXML: %v", err)
	}
	if !strings.Contains(got, "\n  <a>x</a>") || !strings.Contains(got, "\n  <b>y</b>") {
		t.Errorf("PrettyXML did not indent as expected:\n%s", got)
	}
	// Mismatched end tag must error.
	if _, err := PrettyXML([]byte("<a></b>")); err == nil {
		t.Errorf("PrettyXML should error on mismatched tags")
	}
}

// TestPrettyXMLStripsInputWhitespace verifies that pre-existing indentation in the input does not leak into the formatted output.
func TestPrettyXMLStripsInputWhitespace(t *testing.T) {
	in := []byte("<root>\n    <a>x</a>\n    <b>y</b>\n</root>")
	got, err := PrettyXML(in)
	if err != nil {
		t.Fatalf("PrettyXML: %v", err)
	}
	if strings.Contains(got, "    <") {
		t.Errorf("input whitespace leaked into output:\n%s", got)
	}
}

// TestIsJSONIsXML verifies content-type sniffing matches JSON/XML variants (incl. vnd.api+json, SOAP) without false positives.
func TestIsJSONIsXML(t *testing.T) {
	for _, ct := range []string{"application/json", "application/vnd.api+json", "APPLICATION/JSON"} {
		if !IsJSON(ct) {
			t.Errorf("IsJSON(%q) = false", ct)
		}
	}
	if IsJSON("text/plain") {
		t.Errorf("IsJSON false-positive")
	}
	for _, ct := range []string{"application/xml", "text/xml", "application/soap+xml"} {
		if !IsXML(ct) {
			t.Errorf("IsXML(%q) = false", ct)
		}
	}
	if IsXML("application/json") {
		t.Errorf("IsXML false-positive")
	}
}

// TestFormatHeaders verifies that headers are emitted sorted by name with one line per value.
func TestFormatHeaders(t *testing.T) {
	h := http.Header{
		"Content-Type": {"application/json"},
		"Set-Cookie":   {"a=1", "b=2"},
		"X-A":          {"1"},
	}
	want := "Content-Type: application/json\nSet-Cookie: a=1\nSet-Cookie: b=2\nX-A: 1\n"
	if got := FormatHeaders(h); got != want {
		t.Errorf("FormatHeaders = %q\n want %q", got, want)
	}
}

// TestHumanSize verifies the binary-unit formatter at each B/KB/MB/GB boundary.
func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024 * 2, "2.0 GB"},
	}
	for _, c := range cases {
		if got := HumanSize(c.in); got != c.want {
			t.Errorf("HumanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHumanDuration verifies duration formatting across the sub-second, sub-minute, and minute+ ranges.
func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0 ms"},
		{50 * time.Millisecond, "50 ms"},
		{999 * time.Millisecond, "999 ms"},
		{1500 * time.Millisecond, "1.50 s"},
		{65 * time.Second, "1m 5s"},
	}
	for _, c := range cases {
		if got := HumanDuration(c.in); got != c.want {
			t.Errorf("HumanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
