## Summary

What does this change and why?

## Related issues

Fixes #
<!-- or: Refs # -->

## Testing

How did you verify it? (paste relevant `go test ./... -race` output if useful)

## Checklist

- [ ] Behaviour change is paired with tests (failing before, passing after)
- [ ] `go test ./... -race` passes locally
- [ ] `make lint` (golangci-lint) passes
- [ ] Touched non-`internal/ui` packages stay ≥ 90% coverage
- [ ] Docs updated (module `README`/`STRUCTURE`/`WORKFLOW`) where the change
      adds/renames exported identifiers, files, or runtime flows
- [ ] Commit authored as `Bartosz Pachołek <bartosz+github@idct.tech>` with no
      `Co-Authored-By` trailer
