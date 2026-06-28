# HUMANS.md

For human contributors. The [README.md](README.md) covers what Helena is
and how to install and run it; this file covers contributing.

## Why Helena exists

Postman and Bruno are great, but they ship a browser engine inside their
binaries (~200–400 MB on disk) and store collections in formats that don't
diff cleanly. Helena chooses three trade-offs in the other direction:

1. **Native, no Electron** — Fyne renders the UI through OpenGL; the binary
   is ~35 MB and starts instantly.
2. **Open Collection YAML** — plain files that diff and merge like any
   other source code. The spec is at
   https://docs.usebruno.com/opencollection-yaml.
3. **Boring, debuggable Go** — no JS sandbox to babysit, no fancy DI
   framework, no exotic concurrency. The whole runtime is `go test`-able
   without a display except for the UI smoke tests, which use Fyne's
   headless test harness.

If you find yourself reaching for an abstraction that fights one of those
three trade-offs, pause and ask whether the trade-off should be revisited
before adding the abstraction.

## First-time setup

Beyond what's in the README:

- **WSL2 + Windows simultaneously is supported** but you need to pick one
  side per build. Helena's Linux binary built in WSL won't run on Windows
  and vice versa. The CI matrix is the source of truth for release builds.
- **Linux GUI deps**: `sudo apt-get install -y libgl1-mesa-dev xorg-dev`
  even on WSL2 — Fyne's cgo links against system OpenGL/X11 headers.
- **Windows toolchain**: TDM-GCC or MSYS2 mingw-w64 must be on PATH for
  cgo to work. The `windows-latest` GitHub runner has this preinstalled.
- **No `fyne-cross`.** Cross-compiling Fyne with cgo is fragile and brittle
  enough that we sidestep it entirely by using native CI runners.
- **`make.bat`** in the repo root mirrors the Makefile targets for Windows
  cmd / PowerShell.

## Walking through the codebase the first time

A useful tour:

1. [README.md](README.md) — what Helena does.
2. [AGENTS.md](AGENTS.md) — the hard invariants section. Even if you're
   a human, those are the rules.
3. [internal/model/](internal/model/) — the domain types. Everything else
   refers to these.
4. [internal/storage/WORKFLOW.md](internal/storage/WORKFLOW.md) — the
   single most important file in the repo. The `Extra` round-trip is the
   invariant that lets Helena coexist with other OpenCollection tools.
5. [internal/httpclient/WORKFLOW.md](internal/httpclient/WORKFLOW.md) —
   the 11-step request lifetime.
6. [internal/ui/WORKFLOW.md](internal/ui/WORKFLOW.md) — how the editor
   wires up, the `m.loading` flag, the off-UI-goroutine send.

## Adding a feature

A typical full-stack change touches the layers in this order:

1. **`internal/model`** — add the field/type. Keep tags lean: `json` only
   on the model, `yaml` lives on the storage DTO.
2. **`internal/storage/opencollection.go`** — add the matching field to
   the relevant DTO, and update `requestToFile` / `fileToRequest` (or
   the equivalent converter) to copy it.
3. **Test the round-trip** in `internal/storage/storage_test.go`. A test
   that asserts the YAML carries the expected key catches accidental
   regressions to Extra-only preservation.
4. **`internal/session`** if the runtime state needs new accessors.
5. **`internal/ui`** — add the widget, wire `OnChanged` with the
   `!m.loading` guard, populate from `loadRequest`, clear on nil-request.
   Add a UI smoke test in the matching `_test.go`.
6. **Update the module's `STRUCTURE.md`** with the new type or field.
7. **Update the module's `WORKFLOW.md`** if you added a new flow.

For a worked example, see commits adding Phase 7.2 (per-request docs):
the field landed in model, then storage with a round-trip test, then UI
with edit/preview subtabs and write-back tests, with both module docs
updated.

## Documentation is part of the change

The per-module docs (`README.md` / `STRUCTURE.md` / `WORKFLOW.md`) and the
top-level files (`AGENTS.md`, `CLAUDE.md`, this file, `README.md`) are
maintained alongside the code, not as a separate pass. A change that
updates code but leaves the docs describing the old behaviour is incomplete.

Concretely:

- Renamed or added an exported type, function, or method → update the
  module's `STRUCTURE.md` and `README.md`.
- Added a new file, or removed one → update the module's `STRUCTURE.md`
  files table.
- Introduced a new runtime flow (or meaningfully changed an existing one)
  → update the module's `WORKFLOW.md`.
- Added a new module → create all three files at its root and add a row
  to the module map in [AGENTS.md](AGENTS.md).
- Changed an invariant or a project-wide convention → update
  [AGENTS.md](AGENTS.md) and mirror the change in [CLAUDE.md](CLAUDE.md)
  / this file when relevant.
- Shipped, removed, or visibly changed a user-facing feature → update the
  project website in [website/](website/) so its feature list, roadmap,
  and examples stay truthful. When the UI changed, regenerate the
  captures with `make screenshots` (and `make screenshots-fancy` for the
  hero-box art); preview with `make website`. A stale website is a
  user-visible bug, not a cosmetic nit.

The doc-quality bar is "scannable in 30 seconds." Brief beats verbose, and
a one-sentence flow that captures the essential transition beats a
paragraph that paraphrases the code.

## Tests are part of the change

Same rule as docs: when behaviour changes, the tests change in the
same commit. Bug fixes ship with a regression test that fails on the
unfixed code; new features ship with tests that fail before the change
and pass after; signature changes update every callsite's test, not
just the closest one. Before declaring the change done, run
`go test ./... -race` and make sure nothing else broke — a failing
test you didn't cause is still your problem to surface.

From Phase 8 onward the coverage floor for every internal package
outside `internal/ui` is ≥ 90% line coverage. If your change pushes a
package below the floor, fill the gap in the same PR or document why
the new code is genuinely untestable. UI tests are deferred to Phase
11; until then, `internal/ui` is excluded from the gate. See
[AGENTS.md](AGENTS.md) "Keep the tests in sync" for the full trigger
list.

## Commit and PR style

This is a small project; the bar is "would I want to read this commit in
six months?" rather than a formal process.

- **Author** as yourself. The owner has a standing rule about commits
  authored as Bartosz with no AI-coauthor trailer, but that applies only
  to AI-generated commits.
- **Subject** under 70 chars. Imperative mood ("Add docs tab", not
  "Added docs tab" or "Adding docs tab").
- **Body** is optional for small changes but valued for anything with a
  non-obvious WHY.
- **One logical change per commit** when feasible. Documentation passes
  and refactors are reasonable to bundle; mixing a behaviour change with
  a refactor is not.
- **No `Co-Authored-By: Claude`** trailers — see [CLAUDE.md](CLAUDE.md).

PRs follow the same logic. Use the GitHub Actions matrix as your sanity
check — if both `ubuntu-latest` and `windows-latest` go green, the change
is portable.

## Where conversation lives

- **Plan and shaping**: the Asana project at
  https://app.asana.com/1/1214897106264347/project/1215180905395792.
  Each phase task carries its design notes and test plan.
- **Per-module decisions**: the module's own `README` / `STRUCTURE` /
  `WORKFLOW` documents.
- **Issues and feature requests**: GitHub Issues at
  https://github.com/idct/helena/issues.

## License

BSD 4-Clause — see [LICENSE](LICENSE). Contributions are accepted under
the same terms.
