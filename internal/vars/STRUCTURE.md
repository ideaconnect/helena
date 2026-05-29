# vars — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level godoc only. |
| [vars.go](vars.go) | The `Resolver` type, regex, fixed-point loop, and missing-name collector. |
| [vars_test.go](vars_test.go) | Tests for plain substitution, whitespace tolerance, precedence stacking, chained refs, missing-name reporting, and cycle termination. |

## Type catalog

### `Resolver` — [vars.go:17](vars.go#L17)
A wrapper over an ordered slice of scope maps.
- `scopes` (unexported) — passed to `New` low-to-high; `Lookup` scans from the top of the stack down, so the last scope wins.

## Non-trivial internals

### `varRe` — [vars.go:9](vars.go#L9)
Regex `\{\{\s*([^{}]*?)\s*\}\}` — matches `{{name}}` allowing inner whitespace, captures the name without `{}` characters so nested templates can't confuse the matcher.

### `maxPasses` — [vars.go:12](vars.go#L12)
Caps fixed-point iteration at 10. Beyond chained refs, a cycle (`a -> b -> a`) would otherwise spin forever; the cap forces termination and the resulting string is then reported through `unresolvedNames`.

### `substituteOnce` — [vars.go:51](vars.go#L51)
One pass of regex replacement. Unknown names and empty `{{}}` are passed through unchanged so the outer fixed-point loop notices and stops.

### `unresolvedNames` — [vars.go:64](vars.go#L64)
After substitution settles, scans the final string for any leftover `{{name}}` and returns each unique name once in first-seen order — the UI uses this to surface "unresolved" badges.
