# responsefmt

Display helpers for HTTP responses. The package turns raw bytes and `http.Header` values into what Helena's UI shows on the Response tabs: a colored Pretty view (token stream), a foldable Structured tree, a sorted header dump, and short human-readable sizes/durations.

There is no state — every function is pure and operates on its arguments only. The package sits between `httpclient` (which produces the raw response) and `ui` (which renders it).

## Public API

### Functions
- `PrettyJSON(body []byte) (string, error)` — re-indents a JSON document with two-space indentation; errors on invalid JSON.
- `PrettyXML(body []byte) (string, error)` — re-indents an XML document with two-space indentation, stripping pre-existing whitespace-only text nodes.
- `HighlightJSON(s string) []Token` — tokenizes already-valid JSON text into colored runs for the Pretty view. Concatenating every `Token.Text` reproduces the input verbatim.
- `HighlightXML(s string) []Token` — the XML analogue: tags, attribute names/values, text, and comment/CDATA/PI spans each get their own kind, same lossless invariant.
- `ParseJSON(body []byte) (*Node, error)` — builds an ordered Structured tree from a JSON document, preserving object key order; errors on invalid JSON or trailing data.
- `ParseXML(body []byte) (*Node, error)` — builds a Structured tree from an XML document; errors on malformed XML or a document with no elements.
- `IsJSON(contentType string) bool` — case-insensitive sniff for any `*json*` MIME (covers `application/json`, `application/vnd.api+json`, etc.).
- `IsXML(contentType string) bool` — case-insensitive sniff for any `*xml*` MIME (covers `application/xml`, `text/xml`, `application/soap+xml`).
- `FormatHeaders(h http.Header) string` — renders headers as sorted `Key: value` lines, one line per value.
- `HumanSize(n int64) string` — formats a byte count in binary units (`B`, `KB`, `MB`, `GB`).
- `HumanDuration(d time.Duration) string` — formats a duration as `N ms` / `X.XX s` / `Mm Ss`.

### Types
- `Token{Text string; Kind TokenKind}` — one colored run for the Pretty view.
- `TokenKind` — color class: `TokenPlain`, `TokenPunct`, `TokenKey`, `TokenString`, `TokenNumber`, `TokenBool`, `TokenNull`, `TokenTag`, `TokenAttr`, `TokenComment`. Shared by the Pretty highlighter and the Structured tree so a key reads the same color in both views.
- `Node{ID, Label string; LabelKind TokenKind; Value string; Kind TokenKind; Children []*Node}` — one row of the Structured tree. Containers carry `Children` and a count summary `Value` (`{3}` / `[5]`); leaves carry a scalar `Value` colored by `Kind`.

## Dependencies

### Internal
None.

### External (standard library only)
- `bytes`, `encoding/json`, `encoding/xml`, `io` — pretty-printing implementations.
- `fmt`, `strings`, `sort` — formatting and header sorting.
- `net/http` — `http.Header` parameter type.
- `time` — `time.Duration` parameter type.
