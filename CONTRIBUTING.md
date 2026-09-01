# Contributing to Helena

Thanks for your interest in Helena — a free, open-source, devs-for-devs API
client. Contributions of all sizes are welcome.

## Getting set up

See the **Build from source** section of the [README](README.md) for the
prerequisites (Go, a C compiler, and the platform GUI deps). Then:

```sh
make tidy    # resolve modules
make test    # go test ./... -race
make run     # launch the app
make lint    # golangci-lint (a CI gate)
```

The Go toolchain is pinned in `go.mod` via a full-patch `go` directive
(`go 1.26.7`); `go` selects it automatically.

## Project conventions (please read before a non-trivial change)

[AGENTS.md](AGENTS.md) is required reading — it documents the hard invariants
(the storage `Extra` round-trip, CORS-as-advisory, the off-UI-goroutine +
`fyne.Do` rule, no fyne-cross, the module map). [CLAUDE.md](CLAUDE.md) captures
the same expectations for AI-assisted changes.

Two rules are load-bearing:

- **Tests are part of the change.** New behaviour needs a test that fails
  before and passes after; changed behaviour updates the tests that pinned the
  old behaviour; a bug fix adds a regression test. Run `go test ./... -race`
  before opening a PR.
- **Docs are part of the change.** Every module ships `README.md`,
  `STRUCTURE.md`, and `WORKFLOW.md`. If you add/rename an exported identifier,
  add a file, introduce a runtime flow, or relax an invariant, update the
  matching docs in the same change.

### Coverage floor

Every internal package outside `internal/ui` is expected to stay **≥ 90% line
coverage**, enforced in CI by `cmd/covergate`. If a change would drop a package
below the floor, add the missing tests or explain in the PR why the new code is
genuinely untestable.

### Linting & vulnerabilities

CI gates on `golangci-lint` and `govulncheck`. Run `make lint` locally; for the
vulnerability scan, `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.

## Commit identity

All commits in this repo are authored as:

```
Bartosz Pachołek <bartosz+github@idct.tech>
```

Do **not** add `Co-Authored-By` trailers. This is a project-wide convention set
on day one.

## Commit messages & PRs

- Use a short, imperative subject (`module: do the thing`), with a body
  explaining the *why* when it isn't obvious.
- Reference issues with `Fixes #N` / `Refs #N` so they close/link on merge.
- Keep PRs focused; one logical change per PR is easier to review.
- The PR template prompts for a summary, testing notes, and a docs/tests
  checklist — please fill it in.

## Reporting bugs / security

- Functional bugs: open an issue using the **Bug report** template (it asks for
  `helena --version`, your OS, and repro steps).
- Security vulnerabilities: **do not** open a public issue — see
  [SECURITY.md](SECURITY.md).
