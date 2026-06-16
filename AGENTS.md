# AGENTS.md

Guidance for AI assistants (Claude Code, Codex, Cursor, others) joining the
Helena codebase. Read this once at the start of a session, then read the
target module's own docs before changing code.

## What Helena is

A super-lightweight cross-platform API client written in Go + Fyne — a
native alternative to Postman and Bruno, no Electron. Single self-contained
binary on Linux and Windows; macOS deferred. Module path:
`github.com/idct/helena`.

## How to find what you need

Every module directory carries three documents:

| File | Read it when |
| --- | --- |
| `README.md` | First contact with the module. Purpose, public API, dependencies. |
| `STRUCTURE.md` | Looking up a file or type. Files table + type catalog. |
| `WORKFLOW.md` | Tracing runtime flows — life of a request, save sequence, etc. |

Top-level orientation:

| Path | What lives there |
| --- | --- |
| [cmd/helena/](cmd/helena/) | Application entrypoint. |
| [internal/model/](internal/model/) | Domain types shared by every layer. |
| [internal/storage/](internal/storage/) | Open Collection YAML load/save. |
| [internal/vars/](internal/vars/) | `{{variable}}` resolver. |
| [internal/httpclient/](internal/httpclient/) | Request execution, CORS advisory. |
| [internal/auth/](internal/auth/) | Auth inheritance resolution + Apply on outgoing requests. |
| [internal/scripting/](internal/scripting/) | goja JS runtime for per-request pre/post hooks. Mutable `request` in pre, read-only `request` + parsed `response` in post, `helena.env.*` overlay writes, `chain.<alias>` predecessor views. |
| [internal/chain/](internal/chain/) | Per-request before-hooks runner. Recursive resolution with per-request alias scope, cycle detection, and an Executor/RequestFinder seam so the package stays free of httpclient/scripting/session deps. |
| [internal/responsefmt/](internal/responsefmt/) | JSON/XML pretty-print (request-body validate/format) + header / size / duration formatting. |
| [internal/importer/](internal/importer/) | OpenAPI / Swagger / WSDL + URL fetch. |
| [internal/exporter/](internal/exporter/) | cURL / wget rendering. |
| [internal/config/](internal/config/) | Persisted settings + UI state. |
| [internal/session/](internal/session/) | Runtime workspace state, tree, env. |
| [internal/ui/](internal/ui/) | Fyne views and actions. |
| [examples/](examples/) | Bundled sample collection + smoke test; `sample.go` embeds it (`//go:embed httpbin`) and `WriteSample` materializes it for the in-app Load-sample action. |
| [assets/](assets/) | `go:embed`-ed app icon. |
| [FyneApp.toml](FyneApp.toml) | App metadata (Name/ID/Version/Icon) for Fyne's native packaging tools. `ID` must match `cmd/helena`'s `appID` (test-guarded). |
| [.github/workflows/](.github/workflows/) | Native Linux + Windows CI. |

The plan of record is in
[Asana](https://app.asana.com/1/1214897106264347/project/1215180905395792).
Don't recreate decisions captured there — read the task notes first.

## Hard invariants — do not regress

1. **Storage `Extra` round-trip.** Every OpenCollection DTO embeds
   `Extra map[string]yaml.Node \`yaml:",inline"\``. `Save` reads the existing
   file before writing and copies `Extra` from the old DTO into the new one
   so externally-authored fields (auth blocks, runtime scripts, custom keys
   on headers/params) survive edits bit-for-bit. See
   [internal/storage/WORKFLOW.md](internal/storage/WORKFLOW.md). Never add a
   write path that skips the read-existing-first pattern.
2. **CORS is advisory, not a toggle.** A native client cannot enforce CORS;
   `httpclient.corsAdvisory` compares request `Origin` against response
   `Access-Control-Allow-Origin` and surfaces a warning. The request is sent
   regardless. Do not add a "CORS enforcement" path.
3. **UI `m.loading` flag.** `loadRequest` sets `m.loading = true` while it
   pushes values into widgets so the widgets' `OnChanged` callbacks don't
   write the loaded values back into `currentRequest`. Any new widget added
   to the request editor must respect this flag in its write-back closure.
   See [internal/ui/WORKFLOW.md](internal/ui/WORKFLOW.md).
4. **`Send` runs off the UI goroutine.** Network I/O happens inside a
   `go func()` and marshals back to the UI via `fyne.Do(...)`. Never touch
   a Fyne widget from a non-UI goroutine without `fyne.Do`.
5. **Open Collection YAML is the storage format.** Not Bruno's `.bru` DSL.
   The spec lives at https://docs.usebruno.com/opencollection-yaml.
6. **Variable resolution happens once, up front, in `httpclient.Build`.**
   Downstream code sees concrete strings. Missing variables are accumulated
   into one error listing every unresolved name.
7. **Window target is Windows amd64.** Not 386.
8. **No `fyne-cross` / Docker.** CI uses native runners
   (`ubuntu-latest` + `windows-latest`); each binary is built by its own
   OS's cgo toolchain. Don't reintroduce cross-compilation.
9. **Session-scoped env overlay for future scripts.** When scripting lands
   (task 7.3), `helena.env.set(...)` writes to a session overlay, never to
   the on-disk env file. Don't quietly persist script-set vars.
10. **Auth `Inherit` is the default; `None` is explicit.** The zero
    `model.Auth` value is treated as `Inherit` on load so new requests pick
    up their parent's auth. To deliberately suppress inheritance, set
    `Type: AuthNone` — never rely on the zero value to mean "no auth".
    Collection roots default to `None` because they have no parent.
    User-set `Authorization` headers always win over Apply.
11. **OAuth2 token cache is in-memory only and namespaced.** The
    `auth.TokenCache` lives on the `Session` for the process lifetime —
    no persistence. Cache keys are namespaced by collection directory so
    two collections sharing a token URL never share tokens. The user's
    TLS / timeout settings deliberately don't apply to the OAuth2 token
    endpoint (those are for the API under test). Persisting tokens
    requires an encryption story; that's in the backlog.
12. **OAuth2 authorization_code is interactive — never headless.** The
    flow binds an ephemeral `127.0.0.1:0` listener, opens the user's
    browser via `AuthCodeStarter`, and waits up to 5 minutes for the
    redirect. Redirect URI hosts other than `localhost` / `127.0.0.1` /
    `::1` are rejected. The starter lives behind an interface so the
    auth package stays Fyne-free; UI plugs in the real adapter via
    `newAuthCodeStarter()`. Don't make the resolver call OpenURL
    directly — it would couple `internal/auth` to the UI toolkit and
    break the test harness.
13. **Per-request scripts are sandboxed goja runtimes.** Every
    `scripting.Run*` call constructs a fresh `goja.Runtime` so state
    never leaks between requests, and every call is capped at
    `scripting.ScriptTimeout` (5 s) wall-clock via `vm.Interrupt`. A
    script's mutable side effects are limited to (a) the session env
    overlay via `helena.env.set` — invariant 9 — and (b) the in-flight
    request in the pre-request phase via the `request` global. Scripts
    have NO direct filesystem or process-spawn surface, and no goja
    binding that opens its own sockets — keep it that way. Scripts
    DO direct the user's own HTTP client (via `request.url` and
    `request.headers`); this is the same trust model Postman / Bruno
    ship, but it means imported collections are executable, and that
    threat model is documented in
    [internal/scripting/README.md](internal/scripting/README.md). The
    full script-side API is documented there too; future bindings
    should be reviewed against this invariant before they land. The
    runtime is decoupled from `internal/session` and
    `internal/httpclient` behind the `EnvBridge` and `ResponseInput`
    boundaries, so adding a binding that drags either dependency into
    the package is a regression.
14. **Chain execution is single-goroutine serial; alias scope is
    per-request.** The chain runner walks each request's
    `Chain []ChainStep` depth-first, calling `Executor.ExecuteOnce`
    once per step before moving on. Concurrency is sequential within
    one Send: no parallel chain steps, no shared mutable runner
    state. The `chain.<alias>` map a request's scripts see contains
    **only that request's own declared aliases** — a chain step
    never sees its predecessors' aliases. Cycle detection is by
    `Request.ID` (persisted to `info.id` in YAML when present;
    generated by `storage.Load` and written back on the next Save
    when absent — so IDs survive reloads once the file has been
    saved by Helena at least once). Depth, total step count, and
    cumulative console output are capped at `MaxChainDepth` /
    `MaxChainSteps` / `MaxChainConsoleLines` so an imported
    collection can't turn one click into thousands of HTTP calls.
    Failure of any phase aborts the chain AND rolls back the env
    overlay to its pre-Send snapshot, so a partial chain leaves no
    overlay residue. Auth flattening happens once on the UI thread
    via `Session.SnapshotChainFinder`; chain steps inherit folder /
    collection auth identically to the leaf. Chain refs resolve
    **ID-first**: a `ChainStep.RequestID` (pinned to the target's
    persistent `Request.ID`, written to `info.id` in YAML) is
    consulted before the human-readable path so renames + folder
    moves don't break refs. The path is the fallback and the
    user-visible identifier in errors. Don't reintroduce a
    code path where chain steps run with raw `AuthInherit`, or
    where overlay writes survive a chain failure, or where refs
    resolve by path only — all three undo user-facing contracts.
15. **Three overlapping `View` shapes — by design, don't unify.**
    `chain.View` (the runner's per-step snapshot, raw bytes only),
    `scripting.ChainView` (the goja-bound `chain.<alias>` surface,
    with lazy `json`/`xml` accessors via
    `DefineAccessorProperty`), and `httpclient.Response` (the
    wire-level capture, incl. `RequestURL`/`RequestBody` reflecting
    what actually went out) all carry method / url / body / headers
    in slightly different shapes. The duplication is what keeps
    `internal/chain` free of `internal/scripting` and
    `internal/httpclient` (mirroring invariant 13's seam for the
    scripting runtime). The bridge converters
    (`chainViewToScripting` in [internal/ui/shell.go](internal/ui/shell.go),
    `chainExecutor.ExecuteOnce` in the same file) cost a few field
    assignments per step and are worth it. Collapsing any two of
    these into a single shared type reintroduces the cross-package
    dependency the seams were built to avoid — leaf scripts would
    then drag in `httpclient`, or the chain runner would drag in
    `goja`. Both are regressions.

## Keep the docs in sync

Documentation is part of the change, not an afterthought. The per-module
docs lose value the moment they drift from the code, and an agent's most
common failure mode is updating code without touching the docs that
describe it.

- **Exported identifier added / removed / renamed** → update the module's
  `STRUCTURE.md` type catalog and `README.md` public-API section.
- **New runtime flow, or a meaningful change to an existing flow** →
  update the module's `WORKFLOW.md`.
- **New file in a module** → add a row to that module's `STRUCTURE.md`
  files table.
- **New module added under `internal/`, `cmd/`, or top-level** → create
  `README.md`, `STRUCTURE.md`, `WORKFLOW.md` at its root, and add a row
  to the module map in this file.
- **Invariant added, removed, or relaxed** → update the "Hard invariants"
  list in this file. Mirror the change in [CLAUDE.md](CLAUDE.md) if it
  affects Claude-specific behaviour, and in [HUMANS.md](HUMANS.md) if it
  affects contributor onboarding.
- **Build / test / CI commands change** → update this file and the
  top-level [README.md](README.md).

If you finish a change and the docs still describe the old behaviour,
the change isn't done.

## Keep the tests in sync

Tests are part of the change, like docs. Every behaviour-affecting
change MUST be paired with test work in the same turn — even a "trivial"
fix:

- **New behaviour** → add tests that fail before your change and pass
  after. No new code lands without a test that exercises it (unless
  the surface is pure plumbing covered by an existing integration test
  — call that out explicitly in the PR description).
- **Changed behaviour** → update the tests that pinned the old
  behaviour. If no test pinned it, that's a coverage gap — add one
  now while you're already in the file.
- **Removed behaviour** → delete the matching tests; don't leave
  `t.Skip(...)` or commented-out scenarios. Document the removal in
  the change message.
- **Bug fix** → add a regression test that fails on the unfixed
  code. A bug fix without a regression test is a bug fix waiting to
  recur.
- **Signature change** → update every callsite's test, not just the
  closest one. Use `grep` / agent search to find them all.
- **Run the suite** before declaring done: `go test ./... -race`.
  A failing test you didn't cause is still your problem to surface.

Coverage floor (Phase 8 onward): every internal package outside
`internal/ui` is expected to stay ≥ 90% line coverage; UI is excluded
pending Phase 11. If your change pushes a package below the floor,
either fill the gap or call out why the new code is genuinely
untestable — silent coverage drops are not acceptable.

If you finish a change and the test suite hasn't grown or shifted to
match it, the change isn't done.

## Code conventions

- **Go doc comments on every exported identifier** (type, function, method,
  constant, variable). Start with the identifier name. One or two sentences
  describing WHAT and WHY, not HOW.
- **Document non-trivial unexported helpers** when the WHY isn't obvious.
- **Document tests.** Each `TestX` gets a `// TestX verifies <scenario>.`
  one-liner naming the exact scenario.
- **Inline comments only when the WHY is non-obvious.** Don't restate the
  code. Don't narrate ("first we do X, then we do Y").
- **No emojis** anywhere in code, comments, doc files, or commit messages.
- **Brief over verbose.** A one-sentence flow description beats a paragraph
  that summarizes the same thing.
- **No new abstractions beyond what the task requires.** Three similar
  lines beat a premature helper.
- **No error-handling for impossible cases.** Trust internal code and
  framework guarantees; validate only at system boundaries.

## Build and test

The Makefile (Linux/macOS/WSL) and `make.bat` (Windows) expose identical
targets:

```sh
make tidy    # resolve modules
make run     # run the app
make build   # build ./bin/helena (or bin\helena.exe on Windows)
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -w .
make lint    # golangci-lint (optional)
```

Before declaring a task done: `gofmt -l .` (must be empty), `go vet ./...`,
`go test ./...`, `go build ./...` — all clean.

## Things to avoid

- **Adding heavyweight dependencies** without checking binary size. Helena
  ships ~46 MB after task 7.3 added goja; goja, kin-openapi, and gopher-yaml
  are the only large external deps. Justify any addition. The response Body
  viewer uses `github.com/ideaconnect/go-fyne-pretty-view` (pinned at
  `v0.1.0-alpha`, same author/org); it adds no new heavy module — its only
  non-Fyne need is `golang.org/x/net/html`, already in the tree — but it
  bumped `golang.org/x/{net,sys,text}` minor versions. It bundles the Iconoir
  icon set (MIT) for its toolbar; carry that notice if it links. Any version
  bump of it is a deliberate, tested change.
- **Bypassing the storage Save pattern** (see invariant 1).
- **Touching widgets from non-UI goroutines** without `fyne.Do` (see
  invariant 4).
- **Committing `.claude/`, `bin/`, or `dist/`.** All gitignored.
- **Renaming exported identifiers** without explicit user request — this
  ripples through plans, Asana notes, and external bookmarks.
- **Refactoring while doing feature work.** Land the feature first; file a
  cleanup task afterward.

## Design history

- Asana plan: https://app.asana.com/1/1214897106264347/project/1215180905395792
- LICENSE: BSD 4-Clause.
- Decisions made early (Windows amd64, OpenCollection YAML, CORS advisory,
  native CI) are documented in the top-level [README.md](README.md) and the
  per-module docs. Treat them as load-bearing unless the user explicitly
  reopens them.
