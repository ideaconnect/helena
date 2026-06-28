# CLAUDE.md

Project-specific instructions for Claude Code. Anthropic loads this file
into context automatically when a session starts in this repo. Keep it
short and high-signal — the general project conventions live in
[AGENTS.md](AGENTS.md).

## Commit identity (load-bearing)

All commits in this repo MUST be authored as:

```
Bartosz Pachołek <bartosz+github@idct.tech>
```

Do **not** add `Co-Authored-By: Claude <noreply@anthropic.com>` (or any
other Claude trailer) to commit messages. This was set by the project
owner on day one and applies to every commit, including those made by
Claude on the owner's behalf.

Verify with `git log --format='%an <%ae>' -1` if uncertain. The repo's
existing history is the canonical reference.

## Read AGENTS.md first

[AGENTS.md](AGENTS.md) contains the project's hard invariants (storage
`Extra` round-trip, CORS-as-advisory, `m.loading` write-back flag, off-UI
goroutine + `fyne.Do`, no fyne-cross), code conventions, and the module
map. Treat it as required reading for any non-trivial change.

## Per-module documentation

Every module ships three files at its root:

- `README.md` — purpose, public API, dependencies.
- `STRUCTURE.md` — file map + type catalog.
- `WORKFLOW.md` — runtime flows.

When working in a module, read the relevant `WORKFLOW.md` before changing
the code. When adding to a module, update the relevant `STRUCTURE.md`.

**Docs are part of the change.** If you add or rename an exported
identifier, add a new file, introduce a new runtime flow, or relax an
invariant, update the matching docs in the same turn. A change that
leaves the docs describing the old behaviour is not finished. See the
"Keep the docs in sync" section of [AGENTS.md](AGENTS.md) for the
specific trigger conditions.

## The website is part of the change

The project website in [website/](website/) is the public face of Helena.
When a change ships, removes, or visibly alters a user-facing feature,
update the website in the same turn so it never advertises a feature
Helena lacks or omits one it has: refresh the feature list, roadmap, and
examples, and — when the UI changed — regenerate the captures with `make
screenshots` (plus `make screenshots-fancy` for the hero-box art). Stale
copy or an old screenshot on the site is a user-visible bug, not a
cosmetic nit. Preview locally with `make website`. See the "Keep the docs
in sync" trigger list in [AGENTS.md](AGENTS.md).

## Tests are part of the change

Every behaviour-affecting change MUST be paired with test work in the
same turn:

- **New behaviour** → write a test that fails before the change and
  passes after.
- **Changed behaviour** → update the tests that pinned the old
  behaviour.
- **Bug fix** → add a regression test before / alongside the fix.
- **Signature change** → update every callsite's test, not just the
  closest one.
- **Run the suite** before declaring done: `go test ./... -race`.

Coverage floor (Phase 8 onward): every internal package outside
`internal/ui` is expected to stay ≥ 90% line coverage. If a change
pushes a package below the floor, fill the gap or call out why the new
code is genuinely untestable — silent drops are not acceptable. See
"Keep the tests in sync" in [AGENTS.md](AGENTS.md) for the full
trigger list.

## Auto-memory

Helena project facts may be present in your auto-memory under a
`helena-project` entry. Consult it for prior decisions, but verify against
the current code — memory can drift behind the repo state. The plan of
record is in Asana
(https://app.asana.com/1/1214897106264347/project/1215180905395792).

## When in doubt

Default to terseness: brief sentences, no filler, no emojis, no comments
that restate code. The user prefers a one-line update over a paragraph
and will redirect if more depth is needed.
