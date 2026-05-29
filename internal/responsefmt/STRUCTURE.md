# responsefmt — Structure

## Files

| File | Responsibility |
| --- | --- |
| [responsefmt.go](responsefmt.go) | All formatter functions: JSON/XML pretty-print, content-type sniffers, sorted header serializer, byte and duration formatters. |
| [responsefmt_test.go](responsefmt_test.go) | Unit tests for each formatter, content-type sniffing variants (incl. `vnd.api+json`, SOAP), and unit-boundary cases. |

## Type catalog

The package exports no types. All functions take primitive arguments or `http.Header` / `time.Duration` from the standard library.

## Non-trivial internals

### `PrettyXML` token loop — [responsefmt.go:30](responsefmt.go#L30)
The decoder/encoder pair re-indents from scratch. Pre-existing whitespace-only `CharData` tokens (the indentation the producer already emitted) are dropped so the encoder's own indentation is the only one in the output — without this, indents would compound.

### `HumanSize` and `HumanDuration` boundaries — [responsefmt.go:86](responsefmt.go#L86), [responsefmt.go:102](responsefmt.go#L102)
Thresholds are 1024 (binary) for sizes and 1s / 1m for durations. Below the lowest threshold the value is shown as an integer; above it, one decimal (size) or two decimals (sub-minute durations) keep the string compact.
