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

If your change pushes a non-excluded package below 90%, either fill
the gap in the same PR or document why the new code is genuinely
untestable. See AGENTS.md "Keep the tests in sync".

### Current baseline (snapshot at Phase 8 entry)

Snapshot — refresh with `make coverage` and update this table when it
drifts more than ±1 point.

| Package | Coverage | At floor? |
| --- | --- | --- |
| `internal/auth` | 73.1% | NO — fill in 8.2 |
| `internal/chain` | 91.9% | yes |
| `internal/config` | 65.4% | NO — fill in 8.2 |
| `internal/exporter` | 93.9% | yes |
| `internal/httpclient` | 84.8% | NO — fill in 8.2 |
| `internal/importer` | 80.7% | NO — fill in 8.2 |
| `internal/model` | 79.2% | NO — fill in 8.2 |
| `internal/responsefmt` | 95.8% | yes |
| `internal/scripting` | 87.4% | NO — fill in 8.2 |
| `internal/session` | 76.6% | NO — fill in 8.2 |
| `internal/storage` | 85.1% | NO — fill in 8.2 |
| `internal/vars` | 92.9% | yes |
| `internal/ui` | 40.7% | SKIP (Phase 11) |

Four packages already clear the floor: `chain`, `exporter`,
`responsefmt`, `vars`. The other eight are 8.2's target list.

## Behavioral tests (`features/`, godog) — Phase 8.3

`godog` (Gherkin / Cucumber for Go, closest to Behat) drives end-to-end
scenarios across internal packages without going through the UI.

- Location: `features/` at repo root.
- Run: `go test ./features/...`.
- Steps go through storage → session → chain → httpclient. No UI
  wiring. Network endpoints use `httptest.Server`.

Planned feature files (created during 8.3):

- `send.feature` — happy path + 4 error paths.
- `chain.feature` — auth inheritance, env writes, ID-pinned refs, cycle, caps.
- `persistence.feature` — Save → reload preserves Extras, IDs, chain refs.
- `import_export.feature` — OpenAPI 3 → tree, tree → cURL round-trip.
- `auth.feature` — Basic / Bearer / API-Key / OAuth2 cc + inheritance.

## Integration tests (`integration/`) — Phase 8.4

Tests that exercise pipelines across module boundaries. Sit in a
top-level `integration/` Go package so they can import every
`internal/*` module symmetrically. Created during 8.4.

## Fuzz tests — Phase 8.5

Go's built-in fuzzing (`go test -fuzz`) on the parsing / encoding
surfaces most likely to crash or misbehave under adversarial input:

- `vars.Resolver` template parsing
- `httpclient.buildBody` per BodyType
- `storage` YAML parse paths
- `chain.splitChainPath` segment filtering
- `auth.ResolveValues` substitution

Run a single fuzzer locally:

```sh
go test ./internal/vars -fuzz=FuzzResolveTemplate -fuzztime=30s
```

Corpora committed to `testdata/fuzz/`. CI runs each fuzzer for a short
budget per PR; longer runs nightly.

## Mutation testing — Phase 8.6

`go-mutesting` against the five load-bearing packages:

- `internal/chain`
- `internal/storage`
- `internal/httpclient`
- `internal/scripting`
- `internal/auth`

Target kill ratio: ≥ 80% per package. Run locally:

```sh
make mutation
```

Mutation runs are slow (minutes per package) — not gated on every
PR. CI runs them nightly + on manual dispatch and posts the kill
ratio delta as a PR comment.

## Test data fixtures (`testdata/`) — Phase 8.8

Shared inputs the import / storage / integration tests reference:

- `testdata/openapi/` — sample OpenAPI 3.0 / 3.1 specs
- `testdata/swagger/` — Swagger 2.0 specs
- `testdata/wsdl/` — RPC and document-style WSDL
- `testdata/collections/` — OpenCollection YAML fixtures
- `testdata/responses/` — JSON / XML / multipart bodies
- `testdata/fuzz/` — fuzz corpora (auto-managed by `go test -fuzz`)

Tests reference fixtures by relative path. No fixture should live
inside a single package's own `testdata/` if it's used by more than
one package.

## When tests change

See [AGENTS.md](AGENTS.md) "Keep the tests in sync" for the full
trigger list. The short version:

- New behaviour → test that fails before, passes after.
- Changed behaviour → update the tests that pinned the old behaviour.
- Bug fix → regression test that fails on the unfixed code.
- Signature change → every callsite's test.
- Run the suite before declaring done.
