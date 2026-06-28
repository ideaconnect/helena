# internal/sse — workflow

## Parsing a stream

```
httpclient.Stream  ──response.Body──▶  sse.NewParser(body)
        │                                     │
        │           loop: p.Next()            │
        ▼                                     ▼
   onEvent(ev)  ◀────────────  Event{ID,Type,Data,Retry}
```

`Parser.Next` runs the WHATWG interpretation loop:

1. Read a line (terminator: `\n` / `\r` / `\r\n`).
2. **Blank line** → dispatch:
   - If no data was buffered, reset the event-type + data buffers and keep
     reading (no event is produced).
   - Otherwise trim the trailing `\n` off the data buffer, default the type to
     `message`, and return `Event{ID: lastID, Type, Data, Retry}`.
3. **Comment** (`:`…) → ignore.
4. **Field line** → `splitField`, then:
   - `event` → set the (per-event) type buffer.
   - `data` → append `value` + `\n` to the data buffer.
   - `id` → set the persistent `lastID` (unless the value contains a NUL).
   - `retry` → set the persistent `retry` (when the value is all digits).
5. End of input → return `io.EOF`; a half-built event is discarded.

## Lifecycle / cancellation

The parser itself never blocks beyond the reader. `httpclient.Stream` owns the
lifecycle: it checks `ctx.Err()` between events and the request is
context-bound, so cancelling the context unblocks an in-progress `Next` read and
ends the loop. `Stream` stops early when its `onEvent` callback returns false.
