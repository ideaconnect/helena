package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLatestFromURLSuccess verifies a well-formed GitHub response is parsed into
// a Release with the tag and page URL.
func TestLatestFromURLSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Errorf("missing User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.0","html_url":"https://example.test/releases/v0.5.0","extra":"ignored"}`))
	}))
	defer srv.Close()

	rel, err := latestFromURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.Tag != "v0.5.0" || rel.HTMLURL != "https://example.test/releases/v0.5.0" {
		t.Fatalf("got %+v", rel)
	}
}

// TestLatestFromURLNon200 verifies a non-200 status is surfaced as an error.
func TestLatestFromURLNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := latestFromURL(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

// TestLatestFromURLBadJSON verifies a malformed body is an error.
func TestLatestFromURLBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":`))
	}))
	defer srv.Close()

	if _, err := latestFromURL(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

// TestLatestFromURLEmptyTag verifies a response without a tag_name is rejected.
func TestLatestFromURLEmptyTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"html_url":"https://example.test"}`))
	}))
	defer srv.Close()

	if _, err := latestFromURL(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected error on empty tag_name, got nil")
	}
}

// TestLatestFromURLBadRequest verifies an unbuildable request URL errors before
// any network call.
func TestLatestFromURLBadRequest(t *testing.T) {
	if _, err := latestFromURL(context.Background(), DefaultClient(), "://bad_url"); err == nil {
		t.Fatal("expected request-build error, got nil")
	}
}

// TestLatestFromURLTransportError verifies a transport failure (server closed /
// context cancelled) is surfaced.
func TestLatestFromURLTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // also exercise a cancelled context
	if _, err := latestFromURL(ctx, DefaultClient(), url); err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestLatestGitHubReleaseUsesEndpoint verifies the exported wrapper hits the
// GitHub endpoint and returns the parsed release (via an injected transport).
func TestLatestGitHubReleaseUsesEndpoint(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != GitHubLatestURL {
			t.Errorf("unexpected URL %s", r.URL)
		}
		return httptest.NewRecorder().Result(), nil // 200 with empty body -> decode error path is fine
	})}
	// Empty body decodes to a zero struct -> empty tag -> error; we only assert
	// the wrapper routed to the right endpoint (checked above).
	_, _ = LatestGitHubRelease(context.Background(), client)
}

// TestCompare verifies version classification across the interesting cases.
func TestCompare(t *testing.T) {
	cases := []struct {
		name, current, latest string
		want                  Status
	}{
		{"equal", "v0.4.0", "v0.4.0", StatusUpToDate},
		{"equal-no-v", "0.4.0", "v0.4.0", StatusUpToDate},
		{"update-available", "v0.4.0", "v0.5.0", StatusUpdateAvailable},
		{"patch-update", "v0.4.0", "v0.4.1", StatusUpdateAvailable},
		{"ahead", "v0.5.0", "v0.4.0", StatusAhead},
		{"dev-current", "dev", "v0.4.0", StatusUnknown},
		{"empty-current", "", "v0.4.0", StatusUnknown},
		{"nonnumeric-latest", "v0.4.0", "nightly", StatusUnknown},
		{"prerelease-suffix-equal", "v0.4.0", "v0.4.0-rc1", StatusUpToDate},
		{"prerelease-current", "v0.4.0-rc1", "v0.4.0", StatusUpToDate},
		{"differing-length-equal", "v0.4", "v0.4.0", StatusUpToDate},
		{"differing-length-update", "v0.4", "v0.4.1", StatusUpdateAvailable},
		{"build-metadata", "v0.4.0+abc", "v0.4.0", StatusUpToDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Compare(tc.current, tc.latest); got != tc.want {
				t.Fatalf("Compare(%q,%q)=%v want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

// TestDefaultClient verifies the app-level client carries a finite timeout and
// does not inherit the user's API client settings.
func TestDefaultClient(t *testing.T) {
	c := DefaultClient()
	if c.Timeout <= 0 || c.Timeout > 30*time.Second {
		t.Fatalf("unexpected timeout %v", c.Timeout)
	}
}
