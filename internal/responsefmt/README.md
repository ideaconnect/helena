# responsefmt

Display helpers for HTTP responses. The package turns raw bytes and `http.Header` values into the strings Helena's UI shows on the Response tab: pretty-printed JSON/XML, a sorted header dump, and short human-readable sizes/durations.

There is no state — every function is pure and operates on its arguments only. The package sits between `httpclient` (which produces the raw response) and `ui` (which renders it).

## Public API

### Functions
- `PrettyJSON(body []byte) (string, error)` — re-indents a JSON document with two-space indentation; errors on invalid JSON.
- `PrettyXML(body []byte) (string, error)` — re-indents an XML document with two-space indentation, stripping pre-existing whitespace-only text nodes.
- `IsJSON(contentType string) bool` — case-insensitive sniff for any `*json*` MIME (covers `application/json`, `application/vnd.api+json`, etc.).
- `IsXML(contentType string) bool` — case-insensitive sniff for any `*xml*` MIME (covers `application/xml`, `text/xml`, `application/soap+xml`).
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
