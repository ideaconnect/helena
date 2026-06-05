# responsefmt — Structure

## Files

| File | Responsibility |
| --- | --- |
| [responsefmt.go](responsefmt.go) | Formatter functions: JSON/XML pretty-print (`PrettyJSON` / `PrettyXML`), sorted header serializer (`FormatHeaders`), and byte / duration formatters (`HumanSize` / `HumanDuration`). |
| [responsefmt_test.go](responsefmt_test.go) | Unit tests for each formatter, including unit-boundary cases for sizes/durations and the `PrettyXML` whitespace-stripping behavior. |

> The structured-tree (`structured.go`) and syntax-highlighter (`highlight.go`)
> sources were removed when the response Body viewer moved to the external
> `go-fyne-pretty-view` widget. The package no longer exports any types.

## Non-trivial internals

### `PrettyXML` token loop — [responsefmt.go:30](responsefmt.go#L30)
The decoder/encoder pair re-indents from scratch. Pre-existing whitespace-only `CharData` tokens (the indentation the producer already emitted) are dropped so the encoder's own indentation is the only one in the output — without this, indents would compound.

### `HumanSize` and `HumanDuration` boundaries — [responsefmt.go:76](responsefmt.go#L76), [responsefmt.go:92](responsefmt.go#L92)
Thresholds are 1024 (binary) for sizes and 1s / 1m for durations. Below the lowest threshold the value is shown as an integer; above it, one decimal (size) or two decimals (sub-minute durations) keep the string compact.
