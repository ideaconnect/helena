package httpclient

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestRedactURLStripsCredentials verifies userinfo and the query string are
// masked while scheme/host/path stay intact for diagnosis (#112).
func TestRedactURLStripsCredentials(t *testing.T) {
	got := redactURL("https://alice:s3cret@api.example.com/v1/things?token=leakme&x=1")
	if strings.Contains(got, "s3cret") || strings.Contains(got, "leakme") {
		t.Fatalf("redactURL leaked a secret: %q", got)
	}
	if !strings.Contains(got, "api.example.com") || !strings.Contains(got, "/v1/things") {
		t.Errorf("redactURL dropped the useful host/path: %q", got)
	}
}

// TestRedactURLUnparseable falls back to a fully-redacted marker rather than
// echoing an input it could not parse.
func TestRedactURLUnparseable(t *testing.T) {
	if got := redactURL("ht!tp://%zz"); got != "[redacted url]" {
		t.Errorf("redactURL(%q) = %q, want [redacted url]", "ht!tp://%zz", got)
	}
}

// TestSanitizeDoErrorRedactsURLError rebuilds a *url.Error with the credential
// parts of its URL masked, preserving Op and the inner error.
func TestSanitizeDoErrorRedactsURLError(t *testing.T) {
	in := &url.Error{Op: "Get", URL: "http://u:p@host.test/p?token=leakme", Err: errors.New("dial failed")}
	out := sanitizeDoError(in)
	msg := out.Error()
	if strings.Contains(msg, "leakme") || strings.Contains(msg, "u:p@") {
		t.Errorf("sanitizeDoError leaked credentials: %q", msg)
	}
	if !strings.Contains(msg, "dial failed") || !strings.Contains(msg, "host.test") {
		t.Errorf("sanitizeDoError dropped useful diagnostics: %q", msg)
	}
}

// TestSanitizeDoErrorPassesThroughNonURLError leaves a plain error untouched.
func TestSanitizeDoErrorPassesThroughNonURLError(t *testing.T) {
	in := errors.New("some non-url error")
	if out := sanitizeDoError(in); out != in {
		t.Errorf("sanitizeDoError mangled a non-url error: %v", out)
	}
}

// TestDoNetworkErrorDoesNotLeakCredentials drives the full Do path against an
// unresolvable credentialed URL and asserts the surfaced transport error keeps
// the host but not the userinfo or query secret.
func TestDoNetworkErrorDoesNotLeakCredentials(t *testing.T) {
	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.GET,
		URL:    "http://user:s3cret@helena-nonexistent.invalid/path?token=leakme",
	}
	_, err := c.Do(context.Background(), req, vars.New())
	if err == nil {
		t.Fatal("expected a transport error for an unresolvable host")
	}
	if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "leakme") {
		t.Errorf("Do leaked a credential in its error: %q", err)
	}
}
