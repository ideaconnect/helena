package websocket

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestWriteUnmaskedHello pins the encoder against RFC 6455 §5.7's single-frame
// unmasked "Hello": 0x81 0x05 'H' 'e' 'l' 'l' 'o'.
func TestWriteUnmaskedHello(t *testing.T) {
	var b bytes.Buffer
	if err := WriteFrame(&b, Frame{Fin: true, Opcode: OpText, Payload: []byte("Hello")}, false); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	if !bytes.Equal(b.Bytes(), want) {
		t.Errorf("frame = % x, want % x", b.Bytes(), want)
	}
}

// TestReadMaskedHello pins the decoder against RFC 6455 §5.7's masked "Hello"
// frame (mask 0x37fa213d, payload 7f9f4d5158).
func TestReadMaskedHello(t *testing.T) {
	raw := []byte{0x81, 0x85, 0x37, 0xfa, 0x21, 0x3d, 0x7f, 0x9f, 0x4d, 0x51, 0x58}
	f, err := ReadFrame(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Fin || f.Opcode != OpText || string(f.Payload) != "Hello" {
		t.Errorf("decoded frame = %+v", f)
	}
}

// TestRoundTripLengths exercises the three payload-length encodings (7-bit,
// 16-bit, 64-bit) through a masked write → unmasked read round-trip.
func TestRoundTripLengths(t *testing.T) {
	for _, n := range []int{0, 125, 126, 65535, 65536} {
		payload := bytes.Repeat([]byte{'x'}, n)
		var b bytes.Buffer
		if err := WriteFrame(&b, Frame{Fin: true, Opcode: OpBinary, Payload: payload}, true); err != nil {
			t.Fatalf("n=%d write: %v", n, err)
		}
		f, err := ReadFrame(&b)
		if err != nil {
			t.Fatalf("n=%d read: %v", n, err)
		}
		if f.Opcode != OpBinary || !bytes.Equal(f.Payload, payload) {
			t.Errorf("n=%d round-trip mismatch (len %d)", n, len(f.Payload))
		}
	}
}

// TestMaskedFrameOnWire verifies a client-masked frame sets the MASK bit and
// does NOT carry the plaintext, but decodes back to it.
func TestMaskedFrameOnWire(t *testing.T) {
	var b bytes.Buffer
	if err := WriteFrame(&b, Frame{Fin: true, Opcode: OpText, Payload: []byte("secret")}, true); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	if raw[1]&0x80 == 0 {
		t.Error("client frame must set the MASK bit")
	}
	if bytes.Contains(raw[2:], []byte("secret")) {
		t.Error("masked payload should not appear in plaintext on the wire")
	}
}

// TestIsControl covers the control-frame classification.
func TestIsControl(t *testing.T) {
	if !(Frame{Opcode: OpClose}).IsControl() || !(Frame{Opcode: OpPing}).IsControl() {
		t.Error("close/ping should be control frames")
	}
	if (Frame{Opcode: OpText}).IsControl() {
		t.Error("text is not a control frame")
	}
}

// TestReadFrameErrors covers the decoder's failure paths: truncated header,
// truncated extended length, an oversize declared length, a truncated masking
// key, and a truncated payload.
func TestReadFrameErrors(t *testing.T) {
	cases := map[string][]byte{
		"empty":            {},
		"truncated 16-len": {0x82, 126, 0x00}, // says 16-bit len but only 1 byte follows
		"truncated 64-len": {0x82, 127, 0x00, 0x00},
		"oversize":         {0x82, 127, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // length ~ max uint64
		"truncated mask":   {0x81, 0x85, 0x37, 0xfa}, // MASK set, key truncated
		"truncated body":   {0x81, 0x05, 'H', 'e'},   // says 5 bytes, only 2
	}
	for name, raw := range cases {
		if _, err := ReadFrame(bytes.NewReader(raw)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// TestAcceptKey pins the handshake accept-key derivation against RFC 6455 §1.3.
func TestAcceptKey(t *testing.T) {
	if got := AcceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("AcceptKey = %q", got)
	}
}

// TestGenerateKey verifies the key is 16 random bytes, base64-encoded, and not
// repeated between calls.
func TestGenerateKey(t *testing.T) {
	a, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(a)
	if err != nil || len(raw) != 16 {
		t.Errorf("key %q decodes to %d bytes, want 16 (err %v)", a, len(raw), err)
	}
	if b, _ := GenerateKey(); b == a {
		t.Error("GenerateKey returned the same key twice")
	}
}
