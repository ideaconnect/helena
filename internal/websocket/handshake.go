package websocket

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
)

// acceptGUID is the magic string a server concatenates with the client's
// Sec-WebSocket-Key before hashing (RFC 6455 §1.3 / §4.2.2).
const acceptGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// AcceptKey computes the Sec-WebSocket-Accept value the server must return for a
// given Sec-WebSocket-Key: base64(SHA1(key + GUID)). The client verifies the
// server's response header equals this to confirm a real WebSocket peer.
func AcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(acceptGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// GenerateKey returns a fresh Sec-WebSocket-Key: base64 of 16 random bytes
// (RFC 6455 §4.1).
func GenerateKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}
