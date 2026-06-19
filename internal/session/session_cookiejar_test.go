package session

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
)

// TestCookieJarLivesAtSessionScope verifies the session exposes a single, usable
// jar that survives a reload (#91): cookies are not collection-scoped, so a
// workspace/collection switch must not wipe them.
func TestCookieJarLivesAtSessionScope(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	jar := s.CookieJar()
	if jar == nil {
		t.Fatal("CookieJar() returned nil")
	}
	if jar != s.CookieJar() {
		t.Fatal("CookieJar() must return the same instance each call")
	}

	u, _ := url.Parse("https://example.com/")
	jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "1"}})
	if s.CookieJar().Len() != 1 {
		t.Fatalf("cookie not stored on session jar, Len=%d", s.CookieJar().Len())
	}

	// A reload (workspace/collection churn) must not drop the jar's contents.
	s.reload()
	if s.CookieJar() != jar || s.CookieJar().Len() != 1 {
		t.Fatalf("reload reset the cookie jar (Len=%d)", s.CookieJar().Len())
	}
}
