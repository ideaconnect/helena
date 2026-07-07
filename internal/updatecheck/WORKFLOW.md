# internal/updatecheck — workflow

## Opt-in update check

Triggered only by the user clicking **Check for updates** in the bottom status
bar (see [internal/ui/statusbar.go](../ui/statusbar.go)). There is no automatic,
startup, or background invocation.

1. The UI spins a goroutine off the UI thread and calls
   `LatestGitHubRelease(ctx, DefaultClient())` with a short timeout.
2. `latestFromURL` issues a `GET` to `GitHubLatestURL` with a `User-Agent` and
   the versioned `Accept` header, checks for `200`, and decodes
   `{tag_name, html_url}` (bounded by a 1 MiB `io.LimitReader`).
3. `Compare(currentBuildVersion, release.Tag)` classifies the result
   (`UpToDate` / `UpdateAvailable` / `Ahead` / `Unknown`).
4. The UI marshals the verdict back to the UI goroutine via `fyne.Do` and
   updates the status-bar label (plus a "Download" link to the release page when
   an update is available).

## Why this doesn't breach the privacy guarantee

The single outbound request happens only on an explicit click — the same trust
model as any API request the user chooses to send. Nothing here schedules,
retries in the background, or fires at launch, so Helena still makes no
automatic network traffic and sends no telemetry.
