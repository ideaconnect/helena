# responsefmt — Structure

## Files

| File | Responsibility |
| --- | --- |
| [responsefmt.go](responsefmt.go) | Formatter functions: JSON/XML pretty-print, content-type sniffers, sorted header serializer, byte and duration formatters. |
| [highlight.go](highlight.go) | `Token` / `TokenKind` and the `HighlightJSON` / `HighlightXML` tokenizers that color the Pretty view. Hand-rolled scanners over already-valid text — no external lexer dependency. |
| [structured.go](structured.go) | `Node` and the `ParseJSON` / `ParseXML` ordered-tree builders that back the Structured tab. |
| [responsefmt_test.go](responsefmt_test.go) | Unit tests for each formatter, content-type sniffing variants (incl. `vnd.api+json`, SOAP), and unit-boundary cases. |
| [highlight_test.go](highlight_test.go) | Token-kind classification + the lossless concatenation-equals-input invariant for both tokenizers, incl. escaped quotes and truncated spans. |
| [structured_test.go](structured_test.go) | Tree shape, key-order preservation, scalar kinds, XML attributes/text, and error cases for both parsers. |

## Type catalog

| Type | Kind | Role |
| --- | --- | --- |
| `TokenKind` | `int` enum | Color class shared by the Pretty highlighter and the Structured tree: `TokenPlain`, `TokenPunct`, `TokenKey`, `TokenString`, `TokenNumber`, `TokenBool`, `TokenNull`, `TokenTag`, `TokenAttr`, `TokenComment`. |
| `Token` | struct | One colored run: `Text` + `Kind`. Concatenating every `Token.Text` reproduces the tokenizer input. |
| `Node` | struct | One Structured-tree row: `ID`, `Label` / `LabelKind`, `Value` / `Kind`, `Children`. |

## Non-trivial internals

### Tokenizer lossless invariant — [highlight.go](highlight.go)
`HighlightJSON` / `HighlightXML` never validate; they assume the caller passes the output of `PrettyJSON` / `PrettyXML` (already proven to parse). Every byte of input is emitted in exactly one `Token` — unexpected bytes fall through to `TokenPlain` and truncated strings/spans run to end-of-input — so `concat(tokens) == input` always holds and the UI can render the whole document by walking the slice.

### Ordered JSON parse — [structured.go](structured.go#L34)
`ParseJSON` walks the `encoding/json` token stream (not a map decode) so object key order is preserved in the tree; `UseNumber` keeps numbers as their source text. `ParseXML` keeps an element stack and accumulates `CharData` per node, promoting it to a leaf's `Value` only when the element has no child elements.

### `PrettyXML` token loop — [responsefmt.go:30](responsefmt.go#L30)
The decoder/encoder pair re-indents from scratch. Pre-existing whitespace-only `CharData` tokens (the indentation the producer already emitted) are dropped so the encoder's own indentation is the only one in the output — without this, indents would compound.

### `HumanSize` and `HumanDuration` boundaries — [responsefmt.go:86](responsefmt.go#L86), [responsefmt.go:102](responsefmt.go#L102)
Thresholds are 1024 (binary) for sizes and 1s / 1m for durations. Below the lowest threshold the value is shown as an integer; above it, one decimal (size) or two decimals (sub-minute durations) keep the string compact.
