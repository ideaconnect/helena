// Package websocket implements a minimal RFC 6455 client: the opening-handshake
// helpers and the frame codec (#72). It is hand-rolled on the standard library
// — no third-party WebSocket dependency — to keep Helena's dependency footprint
// lean, the same posture as the from-scratch MD4 used for NTLM.
//
// This file is the frame layer: encode/decode of a single RFC 6455 frame and
// the payload masking clients must apply. Message-level framing (fragmentation,
// control frames) and the net connection live alongside it.
package websocket

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

// Opcodes (RFC 6455 §5.2).
const (
	OpContinuation byte = 0x0
	OpText         byte = 0x1
	OpBinary       byte = 0x2
	OpClose        byte = 0x8
	OpPing         byte = 0x9
	OpPong         byte = 0xA
)

// maxFramePayload caps an inbound frame's declared length so a hostile server
// can't make us allocate unbounded memory (64 MiB is far above any real text
// frame; large transfers should fragment).
const maxFramePayload = 64 << 20

// Frame is one decoded WebSocket frame.
type Frame struct {
	Fin     bool
	Opcode  byte
	Payload []byte
}

// IsControl reports whether the frame is a control frame (close/ping/pong),
// which RFC 6455 §5.5 limits to a 125-byte payload and forbids fragmenting.
func (f Frame) IsControl() bool { return f.Opcode&0x8 != 0 }

// WriteFrame encodes f to w. When mask is true (every client→server frame must
// be masked, §5.3) a fresh random masking key is generated and applied.
func WriteFrame(w io.Writer, f Frame, mask bool) error {
	var head [14]byte
	head[0] = f.Opcode & 0x0f
	if f.Fin {
		head[0] |= 0x80
	}
	n := len(f.Payload)
	i := 2
	switch {
	case n <= 125:
		head[1] = byte(n)
	case n <= 0xffff:
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:], uint16(n))
		i = 4
	default:
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:], uint64(n))
		i = 10
	}

	payload := f.Payload
	if mask {
		head[1] |= 0x80
		var key [4]byte
		if _, err := rand.Read(key[:]); err != nil {
			return fmt.Errorf("websocket: mask key: %w", err)
		}
		copy(head[i:], key[:])
		i += 4
		payload = maskPayload(append([]byte(nil), f.Payload...), key)
	}
	if _, err := w.Write(head[:i]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame decodes one frame from r. A masked server→client frame is unmasked
// transparently. Frames declaring a payload over maxFramePayload are rejected.
func ReadFrame(r io.Reader) (Frame, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Frame{}, err
	}
	f := Frame{Fin: h[0]&0x80 != 0, Opcode: h[0] & 0x0f}
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return Frame{}, err
		}
		n = binary.BigEndian.Uint64(ext[:])
	}
	if n > maxFramePayload {
		return Frame{}, fmt.Errorf("websocket: frame payload %d exceeds cap", n)
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return Frame{}, err
		}
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, err
	}
	if masked {
		payload = maskPayload(payload, key)
	}
	f.Payload = payload
	return f, nil
}

// maskPayload XORs data in place with the 4-byte key (§5.3) and returns it.
func maskPayload(data []byte, key [4]byte) []byte {
	for i := range data {
		data[i] ^= key[i%4]
	}
	return data
}
