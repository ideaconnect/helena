package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestOAuth1SignRFC5849 pins the signing algorithm against the worked example
// in RFC 5849 §3.4.1 (signature independently recomputed via Python hmac). The
// param set deliberately includes a duplicate key (a3) and reserved characters.
func TestOAuth1SignRFC5849(t *testing.T) {
	params := [][2]string{
		{"oauth_consumer_key", "9djdj82h48djs9d2"},
		{"oauth_token", "kkk9d7dh3k39sjv7"},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", "137131201"},
		{"oauth_nonce", "7d8f3e4a"},
		{"b5", "=%3D"}, {"a3", "a"}, {"c@", ""}, {"a2", "r b"}, // query
		{"c2", ""}, {"a3", "2 q"}, // body
	}
	got := oauth1Sign("POST", "http://example.com/request", params, "j49sk3j29djd", "dh893hdasih9")
	want := "r6/TJjbCOr97/+UU0NsvSne7s5g="
	if got != want {
		t.Errorf("oauth1Sign = %q, want %q (RFC 5849 §3.4.1)", got, want)
	}
}

// TestOAuthEncode verifies RFC 3986 percent-encoding of the unreserved set and
// reserved characters.
func TestOAuthEncode(t *testing.T) {
	if got := oauthEncode("a-b_c.d~e f@g"); got != "a-b_c.d~e%20f%40g" {
		t.Errorf("oauthEncode = %q", got)
	}
}

var authParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// TestApplyOAuth1 verifies Apply emits an Authorization: OAuth header with the
// expected fields, that ResolveValues substitutes {{vars}}, and that the
// emitted signature recomputes from the header's own nonce/timestamp.
func TestApplyOAuth1(t *testing.T) {
	a := model.Auth{Type: model.AuthOAuth1, OAuth1: &model.OAuth1Auth{
		ConsumerKey: "{{ck}}", ConsumerSecret: "csecret", Token: "tok", TokenSecret: "tsecret",
	}}
	a = ResolveValues(a, func(s string) string {
		if s == "{{ck}}" {
			return "ck-resolved"
		}
		return s
	})

	req, _ := http.NewRequest(http.MethodGet, "https://api.test/path?x=1", nil)
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "OAuth ") {
		t.Fatalf("Authorization = %q", h)
	}
	fields := map[string]string{}
	for _, m := range authParam.FindAllStringSubmatch(h, -1) {
		fields[m[1]] = m[2]
	}
	if fields["oauth_consumer_key"] != "ck-resolved" {
		t.Errorf("oauth_consumer_key = %q, want ck-resolved", fields["oauth_consumer_key"])
	}
	if fields["oauth_signature_method"] != "HMAC-SHA1" || fields["oauth_version"] != "1.0" {
		t.Errorf("method/version fields = %+v", fields)
	}
	// Recompute the signature from the header's own oauth params + the query.
	params := [][2]string{
		{"oauth_consumer_key", "ck-resolved"},
		{"oauth_nonce", unescapePct(fields["oauth_nonce"])},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", fields["oauth_timestamp"]},
		{"oauth_version", "1.0"},
		{"oauth_token", "tok"},
		{"x", "1"},
	}
	want := oauth1Sign("GET", "https://api.test/path", params, "csecret", "tsecret")
	if unescapePct(fields["oauth_signature"]) != want {
		t.Errorf("oauth_signature does not recompute: header=%q want=%q", unescapePct(fields["oauth_signature"]), want)
	}
}

// TestOAuth1BaseURL verifies default-port stripping and scheme/host lowercasing.
func TestOAuth1BaseURL(t *testing.T) {
	cases := map[string]string{
		"http://Example.com:80/p":  "http://example.com/p",
		"https://API.test:443/x/y": "https://api.test/x/y",
		"https://api.test:8443/p":  "https://api.test:8443/p",
		"http://api.test/path":     "http://api.test/path",
	}
	for in, want := range cases {
		u, _ := url.Parse(in)
		if got := oauth1BaseURL(u); got != want {
			t.Errorf("oauth1BaseURL(%s) = %q, want %q", in, got, want)
		}
	}
}

// TestOAuth1FormBody verifies form bodies are parsed for signing and other body
// types / missing GetBody yield nothing.
func TestOAuth1FormBody(t *testing.T) {
	form := "a=1&b=two"
	req, _ := http.NewRequest(http.MethodPost, "https://x/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(form)), nil }
	got, err := oauth1FormBody(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("form params = %v, want 2", got)
	}

	// JSON body → not signed.
	jreq, _ := http.NewRequest(http.MethodPost, "https://x/", strings.NewReader(`{}`))
	jreq.Header.Set("Content-Type", "application/json")
	jreq.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(`{}`)), nil }
	if p, _ := oauth1FormBody(jreq); p != nil {
		t.Errorf("JSON body should not be signed: %v", p)
	}
}

// TestApplyOAuth1FormBody verifies Apply signs a form-bodied POST (exercising
// the body-param branch) and emits the header.
func TestApplyOAuth1FormBody(t *testing.T) {
	form := "status=hello&extra=1"
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/post", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(form)), nil }
	a := model.Auth{Type: model.AuthOAuth1, OAuth1: &model.OAuth1Auth{ConsumerKey: "ck", ConsumerSecret: "cs"}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "OAuth ") {
		t.Errorf("missing OAuth header: %q", req.Header.Get("Authorization"))
	}
}

// unescapePct reverses oauthEncode for the few header values the test re-signs.
func unescapePct(s string) string {
	r := strings.NewReplacer("%2F", "/", "%2B", "+", "%3D", "=", "%20", " ")
	return r.Replace(s)
}

// TestApplyOAuth1NilAndOverwrite verifies a nil sub-struct is a no-op and an
// existing Authorization header is preserved.
func TestApplyOAuth1NilAndOverwrite(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthOAuth1}, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("nil OAuth1 should not set a header")
	}
	req.Header.Set("Authorization", "preset")
	_ = Apply(context.Background(), req, model.Auth{Type: model.AuthOAuth1, OAuth1: &model.OAuth1Auth{ConsumerKey: "k"}}, nil)
	if req.Header.Get("Authorization") != "preset" {
		t.Error("OAuth1 overwrote an existing Authorization header")
	}
}
