# internal/websocket — workflow

## Opening handshake (planned `Dial`, key math here)

```
client                                            server
  │  GET … HTTP/1.1                                  │
  │  Upgrade: websocket                              │
  │  Connection: Upgrade                             │
  │  Sec-WebSocket-Key: <GenerateKey()>              │
  │  Sec-WebSocket-Version: 13                        │
  │ ───────────────────────────────────────────────▶│
  │                       101 Switching Protocols    │
  │       Sec-WebSocket-Accept: <AcceptKey(key)>     │
  │ ◀───────────────────────────────────────────────│
  │  verify Accept == AcceptKey(sentKey) ───────────┘
```

`AcceptKey(key)` = `base64(SHA1(key + 258EAFA5-…))`. The client computes it over
the key it sent and rejects the connection if the server's
`Sec-WebSocket-Accept` header differs — proof of a real WebSocket peer rather
than a cached/confused HTTP response.

## Frame exchange

After the upgrade, both sides speak frames:

- **Send** (`WriteFrame(w, f, true)`): client frames are always masked — a fresh
  random 4-byte key per frame, XORed over the payload, with the MASK bit set.
- **Receive** (`ReadFrame(r)`): server→client frames are unmasked; the decoder
  reads the header, resolves the 7/16/64-bit length, reads (and, if flagged,
  unmasks) the payload. A declared length over 64 MiB is refused before
  allocating.

A message may span multiple frames (FIN=0 continuation frames); control frames
(close/ping/pong, `IsControl()`) may be interleaved and are capped at 125 bytes.
The message-level reassembly + control handling layer lands with `Conn` in the
next increment.
