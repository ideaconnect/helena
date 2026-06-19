package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/idct/helena/internal/cookiejar"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestCookieJarSetThenSend is the acceptance test for #91: a cookie set by one
// response is replayed on the next request through the same jar, and the server
// can read it back.
func TestCookieJarSetThenSend(t *testing.T) {
	var seen string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "secret", Path: "/"})
			w.WriteHeader(http.StatusOK)
		default:
			if c, err := r.Cookie("sid"); err == nil {
				seen = c.Value
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	jar := cookiejar.New()
	c := New(model.Settings{})
	c.SetCookieJar(jar)

	// First request receives the Set-Cookie.
	if _, err := c.Do(context.Background(), model.Request{URL: ts.URL + "/login"}, vars.New()); err != nil {
		t.Fatalf("login: %v", err)
	}
	if jar.Len() != 1 {
		t.Fatalf("jar should hold the login cookie, Len=%d", jar.Len())
	}
	// Second request through the same jar carries it back.
	if _, err := c.Do(context.Background(), model.Request{URL: ts.URL + "/me"}, vars.New()); err != nil {
		t.Fatalf("me: %v", err)
	}
	if seen != "secret" {
		t.Fatalf("server did not receive the stored cookie, got %q", seen)
	}
}

// TestCookieJarSharedAcrossClients proves the session-scope design: a jar shared
// between two throwaway per-send Clients (as the UI builds them) still replays
// cookies, even though each Client is freshly constructed.
func TestCookieJarSharedAcrossClients(t *testing.T) {
	var seen string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "t", Value: "1", Path: "/"})
			return
		}
		if c, err := r.Cookie("t"); err == nil {
			seen = c.Value
		}
	}))
	defer ts.Close()

	jar := cookiejar.New()

	c1 := New(model.Settings{})
	c1.SetCookieJar(jar)
	if _, err := c1.Do(context.Background(), model.Request{URL: ts.URL + "/set"}, vars.New()); err != nil {
		t.Fatalf("set: %v", err)
	}

	c2 := New(model.Settings{}) // a fresh per-send Client, same shared jar
	c2.SetCookieJar(jar)
	if _, err := c2.Do(context.Background(), model.Request{URL: ts.URL + "/get"}, vars.New()); err != nil {
		t.Fatalf("get: %v", err)
	}
	if seen != "1" {
		t.Fatalf("shared jar did not replay cookie across clients, got %q", seen)
	}
}

// TestNoJarNoCookies confirms the jar is opt-in: without SetCookieJar nothing is
// stored or replayed.
func TestNoJarNoCookies(t *testing.T) {
	sawCookie := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "t", Value: "1", Path: "/"})
			return
		}
		sawCookie = r.Header.Get("Cookie") != ""
	}))
	defer ts.Close()

	c := New(model.Settings{}) // no jar installed
	if _, err := c.Do(context.Background(), model.Request{URL: ts.URL + "/set"}, vars.New()); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := c.Do(context.Background(), model.Request{URL: ts.URL + "/get"}, vars.New()); err != nil {
		t.Fatalf("get: %v", err)
	}
	if sawCookie {
		t.Fatal("a request without a jar must not carry cookies")
	}
}
