# responsefmt

Display helpers for HTTP responses and request bodies. The package turns raw
bytes and `http.Header` values into the strings Helena's UI shows: two-space
pretty-printed JSON/XML (used by the request-body Validate/Format buttons), a
sorted header dump, and short human-readable sizes/durations for the response
status line.

There is no state — every function is pure and operates on its arguments only.

> Structured parsing and syntax highlighting used to live here (`ParseJSON`,
> `ParseXML`, `Node`, `HighlightJSON`/`HighlightXML`, `Token`, and the
> `IsJSON`/`IsXML` content-type sniffers). They were removed when the response
> Body viewer moved to the external `go-fyne-pretty-view` widget, which does its
> own parsing, highlighting and format detection.

## Public API

### Functions
- `PrettyJSON(body []byte) (string, error)` — re-indents a JSON document with two-space indentation; errors on invalid JSON.
- `PrettyXML(body []byte) (string, error)` — re-indents an XML document with two-space indentation, stripping pre-existing whitespace-only text nodes.
- `FormatHeaders(h http.Header) string` — renders headers as sorted `Key: value` lines, one line per value.
- `HumanSize(n int64) string` — formats a byte count in binary units (`B`, `KB`, `MB`, `GB`).
- `HumanDuration(d time.Duration) string` — formats a duration as `N ms` / `X.XX s` / `Mm Ss`.

## Dependencies

### Internal
None.

### External (standard library only)
- `bytes`, `encoding/json`, `encoding/xml`, `io` — pretty-printing implementations.
- `fmt`, `strings`, `sort` — formatting and header sorting.
- `net/http` — `http.Header` parameter type.
- `time` — `time.Duration` parameter type.
