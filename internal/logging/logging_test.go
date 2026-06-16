package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedactHeaderValue masks credential headers and leaves others alone.
func TestRedactHeaderValue(t *testing.T) {
	cases := []struct {
		name, value, want string
	}{
		{"Authorization", "Bearer s3cr3t", "[redacted]"},
		{"authorization", "Basic abc", "[redacted]"},
		{"X-API-Key", "key-123", "[redacted]"},
		{"Cookie", "sid=xyz", "[redacted]"},
		{"Accept", "application/json", "application/json"},
		{"Content-Type", "text/plain", "text/plain"},
		{"Authorization", "", ""}, // empty stays empty
	}
	for _, c := range cases {
		if got := RedactHeaderValue(c.name, c.value); got != c.want {
			t.Errorf("RedactHeaderValue(%q,%q) = %q, want %q", c.name, c.value, got, c.want)
		}
	}
}

// TestRedactURL strips userinfo + query but keeps host/path.
func TestRedactURL(t *testing.T) {
	got := RedactURL("https://alice:pw@api.example.com/v1/x?token=leak&a=1")
	if strings.Contains(got, "pw") || strings.Contains(got, "leak") || strings.Contains(got, "token=leak") {
		t.Errorf("RedactURL leaked a secret: %q", got)
	}
	if !strings.Contains(got, "api.example.com") || !strings.Contains(got, "/v1/x") {
		t.Errorf("RedactURL dropped host/path: %q", got)
	}
	if RedactURL("ht!tp://%zz") != "[redacted url]" {
		t.Error("unparseable URL not fully redacted")
	}
}

// TestLoggedRequestLineRedacted is the acceptance test: a logged send line
// carries no plaintext credential — the URL and Authorization are redacted
// before they reach the handler (#49).
func TestLoggedRequestLineRedacted(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf, true)
	defer SetOutput(os.Stderr, false)

	L().Info("send",
		"url", RedactURL("https://user:pass@httpbin.org/anything?token=topsecret"),
		"authorization", RedactHeaderValue("Authorization", "Bearer topsecret"),
	)
	out := buf.String()
	for _, leak := range []string{"topsecret", "user:pass", "pass@", "token=topsecret"} {
		if strings.Contains(out, leak) {
			t.Errorf("log line leaked %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "httpbin.org") || !strings.Contains(out, "[redacted]") {
		t.Errorf("log line missing redacted host/marker:\n%s", out)
	}
}

// TestConfigureWritesFile verifies Configure tees to a log file and the close
// func is usable; a bad path surfaces an error but still returns a safe close.
func TestConfigureWritesFile(t *testing.T) {
	defer SetOutput(os.Stderr, false)

	path := filepath.Join(t.TempDir(), "helena.log")
	closeFn, err := Configure(true, path)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	L().Info("hello", "k", "v")
	if err := closeFn(); err != nil {
		t.Errorf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("log file missing entry: %q", data)
	}

	// No file: still returns a usable (no-op) close func and no error.
	closeFn, err = Configure(false, "")
	if err != nil || closeFn() != nil {
		t.Errorf("Configure(no file) = %v / close err", err)
	}

	// Bad path: returns an error and a non-nil close func.
	closeFn, err = Configure(false, filepath.Join(t.TempDir(), "nope", "x.log"))
	if err == nil {
		t.Error("expected an error opening a log file in a missing dir")
	}
	if closeFn == nil || closeFn() != nil {
		t.Error("Configure should return a safe close func even on error")
	}
}
