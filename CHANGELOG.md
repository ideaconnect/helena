# Changelog

All notable changes to Helena are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Helena adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html):
tags are `vMAJOR.MINOR.PATCH`.

- **MAJOR** — incompatible changes to the on-disk format (config / collection
  schema) or to documented behaviour that a user would have to react to.
- **MINOR** — new, backward-compatible functionality.
- **PATCH** — backward-compatible bug fixes.

Pre-1.0, the public surface is still settling; minor versions may include small
breaking changes, called out under **Changed** with a migration note.

Released notes are also generated on GitHub from merged PRs, categorized by
label via [`.github/release.yml`](.github/release.yml).

## [Unreleased]

### Added
- Schema-versioned config with forward migration.
- Configurable response-body size cap (Settings → Max response).
- `helena --version` reporting the build's tag + commit.
- Dedicated environment manager and structured form-body editor.

### Changed
- Token-endpoint and transport errors are redacted of secrets before display.
- `storage.Save` is now atomic (stage-and-swap); a failed save leaves the
  on-disk collection untouched and the in-memory model rolls back to match.
- Sends reuse a per-session HTTP transport (connection-pool reuse).
- Dependencies bumped to latest, notably the response/request body viewer
  `go-fyne-pretty-view` v2.2.0-alpha → v2.3.0-alpha. As a result, pressing
  **Tab** in the request-body or GraphQL-variables editor now inserts a tab
  character instead of moving focus to the next field (use the mouse to leave
  the editor). The pre/post-request scripting engine (goja) also gained
  ES2018 named capture groups, so `$<name>` backreferences and `.groups` work
  in user scripts.

### Performance
- Release builds ship with `-tags no_emoji`, dropping Fyne's bundled colour-emoji
  font (which Fyne parses fresh per theme scope): **~-75 MB resident (≈ -25%)** and
  a ~4 MB smaller binary. Colour-emoji glyphs are unavailable; response text is
  unaffected. Large response bodies are also reclaimed to the OS promptly so memory
  no longer ratchets up across a session. See the README "Memory & rendering" note
  — most remaining memory is the OpenGL driver, which balloons under software
  rendering (VM / RDP / no GPU driver), not Helena's code.

### Security
- Bearer-token, API-key, and Secret environment values are masked in the UI.
- Added `SECURITY.md`; CI gates on `govulncheck`.

<!--
When cutting a release, move the relevant Unreleased entries under a new
`## [vX.Y.Z] - YYYY-MM-DD` heading.
-->
