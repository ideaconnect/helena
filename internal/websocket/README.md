# internal/websocket

A minimal [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) WebSocket
**client**, hand-rolled on the standard library (#72) — no third-party
WebSocket dependency, the same lean-deps posture as the from-scratch MD4 used
for NTLM.

This is the **frame + handshake + connection layer**. It does the wire-format
encode/decode, the opening-handshake key math, and `Dial` + `Conn` (the HTTP
Upgrade over a hijacked TCP/TLS connection, then message read/write with
fragmentation reassembly + automatic ping→pong + close). The UI for
bidirectional messaging builds on top of it (next increment).

## Public API

| Symbol | Purpose |
| --- | --- |
| `Frame` | A decoded frame: `Fin bool`, `Opcode byte`, `Payload []byte`. `IsControl()` flags close/ping/pong. |
| `OpText` / `OpBinary` / `OpClose` / `OpPing` / `OpPong` / `OpContinuation` | RFC 6455 §5.2 opcodes. |
| `WriteFrame(w, f, mask) error` | Encode one frame. With `mask=true` (mandatory for client→server, §5.3) a fresh random masking key is generated and applied. |
| `ReadFrame(r) (Frame, error)` | Decode one frame; a masked server frame is unmasked transparently. Rejects a declared payload over 64 MiB. |
| `AcceptKey(key) string` | The `Sec-WebSocket-Accept` value for a `Sec-WebSocket-Key`: `base64(SHA1(key + GUID))` (§1.3). The client verifies the server echoes this. |
| `GenerateKey() (string, error)` | A fresh `Sec-WebSocket-Key`: base64 of 16 random bytes (§4.1). |
| `Dial(ctx, url, header) (*Conn, error)` | Open a `ws://`/`wss://` connection: TCP/TLS dial + HTTP Upgrade handshake, verifying the server's accept key. A ctx deadline bounds the handshake; cancellation aborts it (the socket is closed, so a server that accepted TCP but never answers can't block the caller forever). |
| `Conn` | An established connection. `WriteMessage(opcode, data)` sends a masked frame; `ReadMessage()` returns the next text/binary message (reassembling continuations — total size capped at 64 MiB, mirroring the per-frame cap — answering pings, ending on close); `Close()` sends a close frame and tears down. Safe for one reader + one writer. |

## Why hand-rolled

Adding a WebSocket library would be the first third-party network dependency in
the send path. The frame format is small and well-specified, so the codec is
~150 lines and fully pinned to the RFC's own examples (the §5.7 "Hello" frames
and the §1.3 accept key). gRPC, by contrast, is *not* hand-rollable on this
footprint and is deliberately left out.

## Dependencies

Standard library only (`bufio`, `context`, `crypto/rand`, `crypto/sha1`,
`crypto/tls`, `encoding/base64`, `encoding/binary`, `fmt`, `io`, `net`,
`net/http`, `net/url`, `strings`, `sync`, `time`). No third-party deps, no Fyne.
