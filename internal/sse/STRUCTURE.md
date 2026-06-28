# internal/sse — structure

## Files

| File | Purpose |
| --- | --- |
| [sse.go](sse.go) | The whole package: the `Event` type, the `Parser` (`NewParser` / `Next`), and the unexported `splitField` + `scanLines` helpers. |
| [sse_test.go](sse_test.go) | Field parsing, multi-line data, comments, `\r` / `\r\n` / `\n` terminators, `retry`, the no-space-after-colon and bare-field-name cases, the blank-line-without-data non-dispatch, and the EOF-doesn't-dispatch rule. |

## Types

### `Event` ([sse.go](sse.go))
- `ID` — the stream's last event id (persists across events until a new `id:`).
- `Type` — the `event:` field; `"message"` when unset.
- `Data` — accumulated `data:` payload, trailing `\n` removed.
- `Retry` — last `retry:` value in milliseconds (0 when never set).

### `Parser` ([sse.go](sse.go))
Holds the `bufio.Scanner` (with the `scanLines` split func and a 1 MiB max line
to tolerate large `data:` payloads) plus the persistent `lastID` and `retry`
buffers. `Next` accumulates fields until a blank line dispatches an event.

## Internal helpers

| Helper | What it does |
| --- | --- |
| `splitField` | Split a line into field name + value at the first colon; strip one leading space; a colon-less line is a field with an empty value. |
| `scanLines` | A `bufio.SplitFunc` treating `\n`, `\r`, and `\r\n` all as line terminators. |

## What is NOT here

- No network. [internal/httpclient](../httpclient/)'s `Stream` opens the request
  and hands the body to `NewParser`.
- No UI / Fyne. The live event view lives in [internal/ui](../ui/).
- No reconnection. `Retry` is surfaced but Helena does not auto-reconnect; a
  caller decides whether to re-open the stream.
