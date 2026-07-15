# internal/pathparam

Substitution for single-brace `{name}` **path parameters** embedded in a
request URL — the counterpart to [`internal/vars`](../vars/), which resolves
double-brace `{{variable}}` templates. Both syntaxes coexist in one URL:

```
{{base_url}}/drs/bag/{bagId}
   ^^ vars template          ^^ path parameter
```

`{{base_url}}` is filled from the collection / environment scopes; `{bagId}`
is filled from the request's own **Path** tab. Every scan here steps over
`{{...}}` spans, so a template's inner name is never mistaken for a path
parameter.

## Public API

| Function | Purpose |
| --- | --- |
| `Names(s string) []string` | The distinct `{name}` tokens in `s`, in first-seen order, skipping `{{templates}}`. Used by the editor to list a request's path parameters. |
| `Apply(s string, lookup func(name string) (value string, ok bool)) string` | Replace each `{name}` with `lookup(name)`; leave the token intact when `ok` is false. Inserted values are never re-scanned. Used by `httpclient.Build` at send time. |

## Semantics

- **`{{...}}` is opaque.** A template span is copied whole; its inner name is
  never returned by `Names` nor filled by `Apply`.
- **Unfilled tokens stay visible.** `Apply` leaves `{name}` in place when the
  lookup declines it, so a forgotten path parameter reaches the wire as a
  literal `{name}` (and shows in the URL preview) rather than collapsing to an
  empty path segment.
- **No re-scan.** A substituted value that itself contains braces cannot spawn
  further substitution — the security boundary matching `vars`' frozen dynamic
  values.
- **Segment-scoped names.** A name ends at a `/`, `?`, `#`, brace, or
  whitespace, so a token lives within a single path segment.

## Dependencies

Standard library only (`strings`). Deliberately free of `model` / `httpclient`
/ `ui` so both the send path and the editor can share it.
