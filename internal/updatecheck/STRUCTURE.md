# internal/updatecheck — structure

## Files

| File | Purpose |
| ---- | ------- |
| [updatecheck.go](updatecheck.go) | The whole package: `Release` / `Status` types; `LatestGitHubRelease` and its testable `latestFromURL` core; `Compare` with the `parseVersion` / `cmpVersion` helpers; `DefaultClient`; and the `GitHubLatestURL` / `GitHubReleasesURL` / `StorePageURL` constants. |
| [updatecheck_test.go](updatecheck_test.go) | httptest-backed fetch tests (success, non-200, malformed JSON, empty `tag_name`, unbuildable URL, transport/cancel error), a `RoundTripper`-injected test of the exported wrapper, the `Compare` case table, and `DefaultClient`. 100% statement coverage. |

## Types

| Type | Role |
| ---- | ---- |
| `Release` | The subset of a GitHub release we surface: `Tag` (e.g. `v0.4.0`) and `HTMLURL` (the release page). |
| `Status` | Verdict of `Compare`: `StatusUnknown`, `StatusUpToDate`, `StatusUpdateAvailable`, `StatusAhead`. |

## Notable functions

- `LatestGitHubRelease(ctx, client)` → `latestFromURL(ctx, client, GitHubLatestURL)`.
- `parseVersion(s)` normalizes `v1.2.3` / `1.2.3-rc1` to `([]int, pre, ok)` —
  `pre` flags a pre-release suffix (which ranks below the same numeric version),
  build metadata (`+meta`) is dropped, non-numeric input is rejected;
  `cmpVersion(a, b)` compares component-wise, padding the shorter slice with zeros.
