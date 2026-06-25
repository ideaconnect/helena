package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestDigestChallengeRetry verifies the client answers a 401 Digest challenge by
// recomputing the response header and re-sending once, and that {{vars}} in the
// credentials resolve (#75).
func TestDigestChallengeRetry(t *testing.T) {
	var firstHadAuth bool
	var secondAuth string
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "" {
			firstHadAuth = false
			w.Header().Set("WWW-Authenticate",
				`Digest realm="test", qop="auth", nonce="abc123", opaque="op"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		secondAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.GET,
		URL:    ts.URL + "/secure",
		Auth:   model.Auth{Type: model.AuthDigest, Digest: &model.DigestAuth{Username: "{{u}}", Password: "pw"}},
	}
	resp, err := c.Do(context.Background(), req, vars.New(map[string]string{"u": "alice"}))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (calls=%d)", resp.StatusCode, calls)
	}
	if calls != 2 {
		t.Errorf("server saw %d calls, want 2 (challenge + retry)", calls)
	}
	if firstHadAuth {
		t.Error("first request should carry no Authorization")
	}
	if !strings.Contains(secondAuth, `Digest username="alice"`) || !strings.Contains(secondAuth, `nonce="abc123"`) {
		t.Errorf("retry Authorization wrong: %q", secondAuth)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("body = %q, want ok", resp.Body)
	}
}

// TestDigestNoChallengeNoRetry verifies a plain 401 without a Digest challenge is
// returned as-is (no infinite retry).
func TestDigestNoChallengeNoRetry(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{Method: model.GET, URL: ts.URL,
		Auth: model.Auth{Type: model.AuthDigest, Digest: &model.DigestAuth{Username: "u", Password: "p"}}}
	resp, err := c.Do(context.Background(), req, vars.New(nil))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want 1 (no Digest challenge → no retry)", calls)
	}
}
