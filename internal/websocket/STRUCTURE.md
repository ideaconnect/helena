# internal/websocket — structure

## Files

| File | Purpose |
| --- | --- |
| [frame.go](frame.go) | Package doc + the frame layer: the `Frame` type and opcodes, `WriteFrame` / `ReadFrame`, and the unexported `maskPayload`. `maxFramePayload` (64 MiB) caps an inbound declared length. |
| [handshake.go](handshake.go) | The opening-handshake key math: `AcceptKey` (server's `Sec-WebSocket-Accept`) and `GenerateKey` (client's `Sec-WebSocket-Key`), plus the `acceptGUID` constant. |
| [conn.go](conn.go) | `Dial` (URL parse → TCP/TLS dial → `writeUpgrade`/`verifyUpgrade` HTTP handshake; ctx cancellation aborts an in-flight handshake by closing the socket via `context.AfterFunc`) and `Conn` (`WriteMessage`, `ReadMessage` with continuation reassembly + inline ping/pong/close, `writeControl`, `Close`). `maxMessageBytes` caps a reassembled message's total size (test seam var). A write mutex serializes frames so a control echo can't interleave a data frame. |
| [conn_test.go](conn_test.go) | End-to-end against a hijacking echo server that speaks the same codec: handshake + masked send / unmasked echo, ping→pong, fragmentation, close→EOF, unsolicited pong, reserved opcode, over-125 ping, extra headers, and the handshake-rejection / bad-accept / missing-upgrade / TLS-failure / default-port paths. |
| [frame_test.go](frame_test.go) | RFC 6455 known-answer tests: the §5.7 unmasked / masked "Hello" frames, payload-length round-trips across the 7/16/64-bit encodings, the MASK-bit + no-plaintext check, control-frame classification, the decoder error paths (truncated header / length / mask / body, oversize length), and the §1.3 accept key + key generation. |

## Frame layout (RFC 6455 §5.2)

```
byte 0:  FIN(1) RSV(3)=0 opcode(4)
byte 1:  MASK(1) payload-len(7)      ; len 126 → next 2 bytes (uint16 BE)
                                     ; len 127 → next 8 bytes (uint64 BE)
[4 bytes masking key, if MASK]
payload (XOR-masked with the key when MASK is set)
```

`WriteFrame` writes the header into a 14-byte scratch array (max header size:
2 + 8 length + 4 mask) and then the payload. `ReadFrame` reads the 2-byte head,
the extended length if present, the mask key if `MASK`, then the payload, and
unmasks in place.

## Not yet here (next increment)

- UI: a bidirectional message panel (a Connect/Send action, the live transcript
  of sent + received messages) wiring `Dial`/`Conn` into the request editor.
