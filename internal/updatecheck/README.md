# internal/updatecheck

Opt-in, **user-initiated** check for a newer Helena release. Everything here runs
only when the user clicks "Check for updates" in the bottom status bar — nothing
polls, schedules, or runs at startup — so it preserves Helena's
no-background-traffic / no-telemetry guarantee. A manual check is a request the
user chose to make, like any API request they send.

The check is deliberately decoupled from [`internal/httpclient`](../httpclient/)
(which carries the user's per-request TLS/timeout settings for the API under
test): an app-level version check uses its own plain client via `DefaultClient`,
mirroring how the OAuth2 token endpoint doesn't inherit those settings.

## Public API

- `LatestGitHubRelease(ctx, *http.Client) (Release, error)` — fetch the latest
  published release (its tag + page URL) from the GitHub REST API. Excludes
  drafts and pre-releases.
- `Compare(current, latest string) Status` — classify the running build against
  the latest tag: `StatusUpToDate`, `StatusUpdateAvailable`, `StatusAhead`, or
  `StatusUnknown` (a non-comparable build such as `dev`). Tolerates a leading
  `v` and a pre-release/build suffix.
- `DefaultClient() *http.Client` — a plain client with a short timeout.
- `Release{Tag, HTMLURL}`, `Status`, and the `GitHubLatestURL` /
  `GitHubReleasesURL` / `StorePageURL` constants.

## Dependencies

Standard library only (`net/http`, `encoding/json`). No Fyne, no internal
dependencies — the package is UI-toolkit-free and fully unit-tested.
