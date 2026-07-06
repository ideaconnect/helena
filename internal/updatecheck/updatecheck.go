// Package updatecheck performs an opt-in, user-initiated check for a newer
// Helena release. Everything here runs ONLY when the user explicitly clicks
// "Check for updates" in the bottom bar — nothing polls, schedules, or runs in
// the background. This keeps Helena's no-background-traffic / no-telemetry
// guarantee intact: the app never phones home on its own; a manual check is a
// request the user chose to make, like any other.
//
// The check is deliberately decoupled from internal/httpclient (which carries
// the user's per-request TLS/timeout settings for the API under test) — an
// app-level version check uses its own plain client, mirroring how the OAuth2
// token endpoint does not inherit those settings.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GitHubLatestURL is the GitHub REST endpoint for the latest published release
// (it excludes drafts and pre-releases). Unauthenticated; the public
// rate limit (60/hr per IP) is ample for a manual click.
const GitHubLatestURL = "https://api.github.com/repos/ideaconnect/helena/releases/latest"

// GitHubReleasesURL is the human-facing releases page, offered as the fallback
// link when a check fails or to view the release directly.
const GitHubReleasesURL = "https://github.com/ideaconnect/helena/releases"

// StorePageURL is Helena's Microsoft Store product page (Store ID 9NWPKK6CTDR1).
// Opened from the Windows-only "View in Store" affordance; the Store shows and
// installs the latest version there (there is no stable public API for the
// Store's current version number, so we link rather than scrape it).
const StorePageURL = "https://apps.microsoft.com/detail/9NWPKK6CTDR1"

// Release is the subset of a GitHub release we surface.
type Release struct {
	// Tag is the release tag, e.g. "v0.4.0".
	Tag string
	// HTMLURL is the release's page on GitHub.
	HTMLURL string
}

// Status classifies the running build against the latest release.
type Status int

const (
	// StatusUnknown means the versions couldn't be compared — typically a local
	// "dev" build, or a tag that isn't plain vMAJOR.MINOR.PATCH.
	StatusUnknown Status = iota
	// StatusUpToDate means the running build equals the latest release.
	StatusUpToDate
	// StatusUpdateAvailable means the latest release is newer than this build.
	StatusUpdateAvailable
	// StatusAhead means this build is newer than the latest release (an
	// unreleased / pre-release build).
	StatusAhead
)

// DefaultClient is a plain HTTP client with a short timeout for the check. It
// intentionally does not use the user's API TLS/timeout settings.
func DefaultClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// LatestGitHubRelease fetches the latest published release from the GitHub API.
// The client is injectable for testing; pass DefaultClient() in production.
func LatestGitHubRelease(ctx context.Context, client *http.Client) (Release, error) {
	return latestFromURL(ctx, client, GitHubLatestURL)
}

// latestFromURL is the testable core of LatestGitHubRelease, parameterized on
// the endpoint so tests can point it at an httptest server.
func latestFromURL(ctx context.Context, client *http.Client, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	// GitHub requires a User-Agent and recommends the versioned Accept header.
	req.Header.Set("User-Agent", "helena-update-check")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain a little so the connection can be reused; body content is unused.
		_, _ = io.CopyN(io.Discard, resp.Body, 512)
		return Release{}, fmt.Errorf("github: unexpected status %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("github: decode response: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("github: response had no tag_name")
	}
	return Release{Tag: payload.TagName, HTMLURL: payload.HTMLURL}, nil
}

// Compare classifies current against latest. Either may carry a leading "v".
// Build metadata ("+meta") is ignored; a pre-release suffix ("-rc1") ranks below
// the same numeric version without one (SemVer §11), so an rc build sees its
// released version as an update. Anything non-numeric (e.g. "dev") yields
// StatusUnknown.
func Compare(current, latest string) Status {
	cur, curPre, okCur := parseVersion(current)
	lat, latPre, okLat := parseVersion(latest)
	if !okCur || !okLat {
		return StatusUnknown
	}
	if c := cmpVersion(cur, lat); c != 0 {
		if c < 0 {
			return StatusUpdateAvailable
		}
		return StatusAhead
	}
	// Equal numeric versions: a pre-release build ranks below the release.
	switch {
	case curPre && !latPre:
		return StatusUpdateAvailable
	case !curPre && latPre:
		return StatusAhead
	default:
		return StatusUpToDate
	}
}

// parseVersion turns "v1.2.3" / "1.2.3-rc1" into ([1,2,3], pre, ok). pre is true
// when a pre-release suffix ("-rc1") is present, which lowers precedence versus
// the same numeric version. Build metadata ("+meta") does not affect precedence
// and is discarded. Missing trailing components default to 0; non-numeric input
// (e.g. "dev") returns ok=false.
func parseVersion(s string) (parts []int, pre bool, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Build metadata never affects precedence — drop it first.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	// A pre-release suffix lowers precedence vs the same numeric version.
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = true
		s = s[:i]
	}
	if s == "" {
		return nil, false, false
	}
	fields := strings.Split(s, ".")
	parts = make([]int, len(fields))
	for i, p := range fields {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false, false
		}
		parts[i] = n
	}
	return parts, pre, true
}

// cmpVersion returns -1 if a < b, 0 if equal, 1 if a > b, comparing
// component-wise and treating missing trailing components as 0.
func cmpVersion(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}
