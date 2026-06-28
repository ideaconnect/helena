# internal/websocket

A minimal [RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455) WebSocket
**client**, hand-rolled on the standard library (#72) — no third-party
WebSocket dependency, the same lean-deps posture as the from-scratch MD4 used
for NTLM.

This is the **frame + handshake layer** (the first increment). It does the
wire-format encode/decode and the opening-handshake key math; the net
connection (`Dial`, message read/write with fragmentation + ping/pong) and the
UI for bidirectional messaging build on top of it.

## Public API

| Symbol | Purpose |
| --- | --- |
| `Frame` | A decoded frame: `Fin bool`, `Opcode byte`, `Payload []byte`. `IsControl()` flags close/ping/pong. |
| `OpText` / `OpBinary` / `OpClose` / `OpPing` / `OpPong` / `OpContinuation` | RFC 6455 §5.2 opcodes. |
| `WriteFrame(w, f, mask) error` | Encode one frame. With `mask=true` (mandatory for client→server, §5.3) a fresh random masking key is generated and applied. |
| `ReadFrame(r) (Frame, error)` | Decode one frame; a masked server frame is unmasked transparently. Rejects a declared payload over 64 MiB. |
| `AcceptKey(key) string` | The `Sec-WebSocket-Accept` value for a `Sec-WebSocket-Key`: `base64(SHA1(key + GUID))` (§1.3). The client verifies the server echoes this. |
| `GenerateKey() (string, error)` | A fresh `Sec-WebSocket-Key`: base64 of 16 random bytes (§4.1). |

## Why hand-rolled

Adding a WebSocket library would be the first third-party network dependency in
the send path. The frame format is small and well-specified, so the codec is
~150 lines and fully pinned to the RFC's own examples (the §5.7 "Hello" frames
and the §1.3 accept key). gRPC, by contrast, is *not* hand-rollable on this
footprint and is deliberately left out.

## Dependencies

Standard library only (`crypto/rand`, `crypto/sha1`, `encoding/base64`,
`encoding/binary`, `fmt`, `io`). No third-party deps, no Fyne.
