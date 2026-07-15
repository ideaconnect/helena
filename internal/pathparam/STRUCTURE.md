# internal/pathparam — structure

## Files

| File | Contents |
| --- | --- |
| `doc.go` | Package doc: purpose and its relationship to `internal/vars`. |
| `pathparam.go` | `Names`, `Apply`, and the shared `walk` scanner. |
| `pathparam_test.go` | Token extraction, substitution, `{{template}}` skipping, and malformed-input behaviour. |

## Types and functions

| Identifier | Kind | Purpose |
| --- | --- | --- |
| `Names(s string) []string` | func | Distinct `{name}` tokens in first-seen order, skipping `{{templates}}`. |
| `Apply(s string, lookup func(name string) (value string, ok bool)) string` | func | Replace each `{name}` via `lookup`; leave the token when `ok` is false; never re-scan inserted values. |
| `isNameByte(b byte) bool` | func (unexported) | Whether a byte may appear inside a path-parameter name (ends at `/ ? # { }` or whitespace). |
| `walk(s string, repl func(name string) string) string` | func (unexported) | Left-to-right scanner shared by `Names`/`Apply`: copies literals + `{{...}}` spans verbatim, hands each `{name}` token to `repl`. |
