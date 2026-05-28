package responsefmt

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

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
