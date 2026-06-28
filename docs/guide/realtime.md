# Real-time — SSE & WebSocket

Helena speaks two real-time protocols in addition to request/response HTTP. Both
are hand-rolled on the Go standard library — no third-party dependency.

## Server-Sent Events (SSE)

A one-way stream of events over HTTP (`text/event-stream`).

1. Enter the endpoint URL as usual.
2. Click the **Stream** button (next to Send).

Helena opens the stream with `Accept: text/event-stream`, reads the body
incrementally, and appends each event to the response **Body** view live, with a
running event count in the status line. The Stream button becomes **Stop** while
the stream is open — click it (or send a new request) to end the stream.

Each event is rendered with its optional `event:` type and `id:` followed by the
`data` payload. The parser follows the WHATWG interpretation algorithm: multi-line
`data:` accumulation, `\n` / `\r` / `\r\n` line endings, comments, and the
persistent last-event-id.

!!! note
    A stream and a normal Send are mutually exclusive — stop one before starting
    the other.

## WebSocket

A persistent, bidirectional connection (RFC 6455).

1. Enter a **`ws://`** or **`wss://`** URL.
2. Click **Send** (or press Enter in the URL field).

Because the URL scheme is `ws`/`wss`, Helena opens a **WebSocket session dialog**
instead of doing an HTTP request:

- It dials the resolved URL and runs the opening handshake (verifying the
  server's `Sec-WebSocket-Accept`). Your request's enabled headers — e.g. an
  `Authorization` header — are carried into the upgrade, with `{{variables}}`
  resolved.
- Received messages appear in the transcript prefixed `←`; messages you type and
  **Send** appear prefixed `→`.
- Pings from the server are answered with pongs automatically; fragmented
  messages are reassembled.
- **Close** the dialog to close the connection.

Text frames are shown as text; binary frames are summarized as
`<N binary bytes>`.

!!! info "Hand-rolled, spec-pinned"
    The RFC 6455 frame codec (masking, the 7/16/64-bit length encodings, control
    frames) and the `Sec-WebSocket-Accept` derivation are implemented from the
    spec and pinned to its own §5.7 and §1.3 examples. gRPC, by contrast, can't
    be hand-rolled on this footprint and is intentionally not bundled.
