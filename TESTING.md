# TESTING.md

How Helena's test suite is organised, how to run it, and what the
coverage / mutation / fuzz gates expect.

The plan of record for the Phase 8 testing pass lives in
[Asana](https://app.asana.com/1/1214897106264347/project/1215180905395792).
This file is the developer-facing companion — what to run, what to
edit, what failure means.

## Canonical commands

| Command | What it does |
| --- | --- |
| `make test` | Run the full suite once. Use this as the smoke test. |
| `make test` *(under `-race`)* — `go test ./... -race` | Race-detector run. CI uses this. Slower (~6 s scripting). |
| `make coverage` | Run tests with `-coverprofile`, print per-package summary. |
| `make coverage-html` | Render the last profile to `coverage.html` for browser inspection. |
| `make coverage-gate` | Run + enforce the per-package floor; non-zero exit on breach. Used by CI. |
| `go test ./<pkg>/ -run TestX -v` | Single test, single package, verbose. |

The Windows equivalents are in `make.bat` (same target names).

## Coverage floor (Phase 8 onward)

Every package outside the excluded set must stay at or above **90%
line coverage**. The exclusion list lives in the Makefile as
`COVERAGE_EXCLUDES`:

| Excluded | Why |
| --- | --- |
| `internal/ui` | UI tests deferred to Phase 11. `fyne/test` is limited and the cost/benefit gates better when scoped separately. |
| `cmd/` | Entrypoints — no behavior worth unit-testing; covered by build + smoke runs. |
| `features/` | Behavioral test harness (godog scenarios + fixtures). Coverage of step definitions and the test world isn't load-bearing; the scenarios themselves are what gate. |
| `integration/` | Cross-package integration tests + shared `Pipeline` helper. Same rationale as `features/` — the scenarios gate, not the test infrastructure's coverage. |

If your change pushes a non-excluded package below 90%, either fill
the gap in the same PR or document why the new code is genuinely
untestable. See AGENTS.md "Keep the tests in sync".

### Current baseline (post 8.2 — all gated packages at the floor)

Snapshot — refresh with `make coverage` and update this table when it
drifts more than ±1 point.

| Package | Coverage | At floor? |
| --- | --- | --- |
| `internal/auth` | 91.0% | yes |
| `internal/chain` | 91.9% | yes |
| `internal/config` | 92.3% | yes |
| `internal/exporter` | 93.9% | yes |
| `internal/httpclient` | 92.9% | yes |
| `internal/importer` | 90.8% | yes |
| `internal/model` | 100.0% | yes |
| `internal/responsefmt` | 95.8% | yes |
| `internal/scripting` | 90.8% | yes |
| `internal/session` | 90.2% | yes |
| `internal/storage` | 90.2% | yes |
| `internal/vars` | 92.9% | yes |
| `internal/ui` | 40.7% | SKIP (Phase 11) |

`make coverage-gate` exits 0. Total internals coverage (excluding UI
and `cmd/`) lands above 92%.

## Behavioral tests (`features/`, godog) — Phase 8.3

`godog` (Gherkin / Cucumber for Go, closest to Behat) drives end-to-end
scenarios across internal packages without going through the UI.

- Location: `features/` at repo root.
- Run: `go test ./features/...`.
- Steps go through storage → session → chain → httpclient → scripting.
  No UI wiring. Network endpoints use `httptest.Server` (per-scenario
  via the `handlerMux` in `features/handlermux.go`).
- Shared scenario state lives in `features/world.go` — own session,
  test server, captured Send result. Each scenario gets a fresh world
  via the godog Before hook; After cleans up the temp dir.

Feature files:

| File | Status | Scenarios |
| --- | --- | --- |
| `send.feature` | **landed** (8.3.1) | 5 scenarios — GET happy path, network error, pre-script URL rewrite, post-script non-fatal error, pre-script fatal error |
| `chain.feature` | planned (8.3.2) | A→B chain, auth inheritance, env writes, ID-pinned refs, cycle, caps |
| `persistence.feature` | planned (8.3.3) | Save → Load preserves Extras / `info.id` / `ChainStep.RequestID` / rename cascade |
| `import_export.feature` | planned (8.3.4) | OpenAPI 3 → tree, tree → cURL |
| `auth.feature` | planned (8.3.5) | Basic / Bearer / API-Key / OAuth2 cc + inheritance |

## Integration tests (`integration/`) — Phase 8.4

Go-native tests that exercise pipelines across module boundaries.
Sit in a top-level `integration/` package so they can import every
`internal/*` module symmetrically.

- Run: `go test ./integration/... -race`.
- Shared `Pipeline` helper (in `integration/pipeline.go`) bundles
  the on-disk collection dir, the session, the test server, and a
  `Send(path)` method that runs the full chain → leaf pipeline the
  UI Send button takes. Each test gets a fresh, isolated Pipeline.
- Coverage of the helper isn't gated; what gates is whether the
  wires hold.

Suites:

- `pipeline_test.go` — full save → reload → send round-trip with
  folder-inherited auth + chain refs + leaf pre-script reading
  `chain.<alias>.response.json`; concurrent Sends race-clean.
- `oauth2_cache_test.go` — token cached within a collection, cache
  keys namespaced across collections, `ClearAll` forces re-fetch.
- `scripts_overlay_test.go` — env overlay write reaches the next
  Send's `{{var}}`; overlay cleared on reopen; chain-failure rolls
  back overlay writes the chain step landed.
- `importer_storage_test.go` — OpenAPI spec → storage → reopen →
  Send the imported requests; chain a fresh request to an imported
  one and use the imported response in the pre-script.

## Fuzz tests — Phase 8.5

Go's built-in fuzzing (`go test -fuzz`) on the parsing / encoding
surfaces most likely to crash or misbehave under adversarial input:

| Package | Target(s) | What it asserts |
| --- | --- | --- |
| `internal/vars` | `FuzzResolveTemplate`, `FuzzResolveAccumulatesMissingPerCallsite` | Never panics; missing list deduped; output bounded. |
| `internal/httpclient` | `FuzzBuildBody`, `FuzzBuildBodyForm` | Every BodyType + Content combo builds without panic; only multipart returns an error; bodyless types produce nil body. |
| `internal/storage` | `FuzzReadCollectionFile`, `FuzzReadRequestFile`, `FuzzLoadCollection` | Adversarial YAML never crashes the parser. |
| `internal/session` | `FuzzSplitChainPath` | Path filter rejects `.`/`..`; no segment contains `/`; idempotent round-trip. |
| `internal/auth` | `FuzzResolveValuesBasic`, `FuzzResolveValuesOAuth2`, `FuzzResolveValuesAPIKey` | Sub-struct never nilled; every field round-trips through an identity resolver; OAuth2 covers all 7 substituted fields. |

Run a single fuzzer locally:

```sh
go test ./internal/vars -fuzz='^FuzzResolveTemplate$' -fuzztime=30s
```

The `^…$` anchor is necessary when a package has more than one
`Fuzz*` target (the `-fuzz` flag matches a single test).

Corpora live under each package's `testdata/fuzz/`. CI runs each
fuzzer for a short budget per PR (set up in 8.7); longer runs
nightly.

## Mutation testing — Phase 8.6

`gremlins` (modern actively-maintained replacement for the legacy
`go-mutesting`) runs against the five load-bearing packages. The
tool is auto-installed on first use via `go install` — no Go
module dep is added.

Run all five sequentially:

```sh
make mutation
```

Or one package at a time during iteration:

```sh
make mutation-chain
make mutation-storage
make mutation-httpclient
make mutation-scripting
make mutation-auth
```

Each target invalidates `go test`'s cache first — gremlins reads
the test cache for baseline timing, and a stale entry causes
spurious timeouts. `--timeout-coefficient 6` gives mutated tests
6× the baseline budget; the chain runner's cap-checks need that
slack when a depth/step-count guard is the mutated line (removing
it triggers infinite recursion that the test framework catches
but reports as TIMED OUT rather than KILLED).

### Baseline (2026-05-30)

Target: ≥ 80% efficacy per package.

| Package | Efficacy | At target? |
| --- | --- | --- |
| `internal/chain` | 85.29% | yes (was 61.76%; 8.6.1 closed) |
| `internal/storage` | 81.25% | yes |
| `internal/httpclient` | 91.67% | yes (was 75.00%; 8.6.2 closed) |
| `internal/scripting` | 87.04% | yes |
| `internal/auth` | 88.35% | yes |

The remaining LIVED mutations are equivalent or near-equivalent (the
same observable behavior with the source flipped — e.g. `if leaf.ID
!= ""` mutations are still caught one recursion level deeper by the
inner `if sub.ID != ""` block, and the visible error is identical;
empty-params boundary `> 0` vs `>= 0` is indistinguishable when the
loop body is a no-op for the same input). Not worth squeezing higher
via increasingly tortured tests.

Three of five clear the bar. The two below get iteration work in
8.6.1 (chain) and 8.6.2 (httpclient).

Mutation runs are slow (10–60 s per package depending on test
suite runtime) — not gated on every PR. 8.7 wires a nightly CI
job that posts the kill-ratio delta as a PR comment.

## Test data fixtures (`testdata/`) — Phase 8.8

Shared inputs the import / storage / integration tests reference.
Each file's purpose is indexed in [testdata/README.md](testdata/README.md).
A smoke test in `integration/testdata_smoke_test.go` parses every
fixture so a typo or stale entry surfaces immediately instead of
breaking a downstream test in a confusing way.

| Subdir | Contents |
| --- | --- |
| `testdata/openapi/` | `minimal.yaml`, `complex.yaml` (parses cleanly), `broken.yaml` (intentionally malformed). |
| `testdata/swagger/` | `basic.yaml` (Swagger 2 minimal), `parameters.yaml` (query / header / path / formData coverage). |
| `testdata/wsdl/` | `rpc.wsdl` (RPC style), `document.wsdl` (document/literal with embedded schema). |
| `testdata/collections/` | `minimal/`, `complex/` (folders + chain + scripts + folder Bearer), `extras/` (hand-authored with `helena-x-*` markers on every Extras-carrying DTO). |
| `testdata/responses/` | `users.json`, `feed.xml`, `login.json` — realistic bodies for scripting + response-format tests. |
| `testdata/fuzz/` *(under each package's own testdata)* | Auto-managed by `go test -fuzz`. |

Tests reference shared fixtures by absolute path resolved from the
repo root (see `repoRoot` in `integration/testdata_smoke_test.go`).
No fixture lives in `testdata/` if it's only used by one package —
that goes under the package's own `testdata/`.

### Invariant 1 gap surfaced + fixed

The `extras/` fixture's per-header, per-param, and per-auth-block
`helena-x-*` markers initially failed to round-trip — exposing a
gap in the Extras-preservation path (only the top-level catch-alls
were merged on save). The fix in `internal/storage/store.go` added
`mergeAuthExtras` + per-row pairing for `mergeKVExtras` /
`mergeParamExtras` so every documented invariant-1 surface is now
exercised on each in-place re-save.

## CI configuration — Phase 8.7

The single workflow [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
runs the following jobs:

| Job | Trigger | Gates? | What it does |
| --- | --- | --- | --- |
| `build` | push, PR | yes | gofmt, vet, `go test ./... -race`, coverage profile, **coverage gate** (Linux), build artifact. Matrix: ubuntu-latest + windows-latest. |
| `fuzz` | PR | yes | Matrix per fuzz target — `go test ... -fuzz=^FuzzX$ -fuzztime=20s`. A fuzz failure (crash / falsified invariant) fails the PR. |
| `mutation` | nightly cron + manual dispatch | report-only | `make mutation` against the 5 load-bearing packages; log uploaded as artifact. Does not block PRs. |
| `release` | tag push | n/a | Downloads build artifacts and publishes a GitHub release. |

The coverage gate uses the same `cmd/covergate` binary as
`make coverage-gate`, so local runs and CI agree on the threshold
(per-package ≥ 90%, excludes documented in TESTING.md above).

The mutation job catches regression-via-weak-test before nightly
results land in inboxes; the artifact carries the per-package
efficacy summary that 8.6.1 / 8.6.2 iterate against.

## When tests change

See [AGENTS.md](AGENTS.md) "Keep the tests in sync" for the full
trigger list. The short version:

- New behaviour → test that fails before, passes after.
- Changed behaviour → update the tests that pinned the old behaviour.
- Bug fix → regression test that fails on the unfixed code.
- Signature change → every callsite's test.
- Run the suite before declaring done.
