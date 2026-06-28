# internal/sse

Parses [Server-Sent Events](https://html.spec.whatwg.org/multipage/server-sent-events.html)
(`text/event-stream`) streams (#74). A pure stream→events transform: no
network, no UI, no Fyne. [internal/httpclient](../httpclient/)'s `Stream`
drives the request and feeds the response body here; the UI renders the
events.

## Public API

| Symbol | Purpose |
| --- | --- |
| `Event` | One dispatched event: `ID`, `Type` (the `event:` field, default `"message"`), `Data` (accumulated `data:` payload, trailing `\n` trimmed), `Retry` (ms). |
| `NewParser(r io.Reader) *Parser` | Wrap an event-stream reader. |
| `(*Parser).Next() (Event, error)` | Return the next dispatched event, or `io.EOF` at end of stream. |

## Behaviour (per the WHATWG algorithm)

- Lines end with `\n`, `\r`, or `\r\n` — any of the three.
- A line starting with `:` is a comment and is ignored.
- `field: value` (one optional leading space stripped); a line with no colon
  is a field name with an empty value.
- Recognised fields: `event`, `data`, `id`, `retry`. Unknown fields are ignored.
- `data:` lines accumulate, joined with `\n`; the trailing `\n` is removed on
  dispatch.
- A **blank line dispatches** the buffered event. A blank line with no buffered
  data resets the in-progress fields **without** dispatching, so `Next` only
  returns events that carry data.
- `id` persists across events (the "last event ID buffer"); an `id` containing
  a NUL is ignored. `retry` (when numeric) persists and rides on every event.
- End of stream (`io.EOF`) does **not** dispatch a half-built event.

## Dependencies

Standard library only (`bufio`, `io`, `strconv`, `strings`). No third-party
deps, no Fyne.
