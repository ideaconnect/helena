package websocket

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Conn is an established client WebSocket connection (#72): the framing layer
// over a hijacked TCP/TLS connection. It is safe for one concurrent reader and
// one concurrent writer (the UI reads on a goroutine and writes from the UI
// thread); a mutex serializes the writes so a control frame (pong/close) can't
// interleave with a data frame on the wire.
type Conn struct {
	netConn net.Conn
	br      *bufio.Reader
	wmu     sync.Mutex
	tls     bool
}

// Dial performs the RFC 6455 opening handshake to a ws:// or wss:// URL and
// returns the established connection. Extra headers (e.g. Authorization,
// subprotocols) are added to the upgrade request. The context bounds the dial
// and the handshake: a deadline caps them, and cancellation aborts an
// in-flight handshake (the UI cancels when its dialog closes mid-dial).
func Dial(ctx context.Context, rawURL string, header http.Header) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("websocket: bad url: %w", err)
	}
	secure := false
	switch strings.ToLower(u.Scheme) {
	case "ws", "http":
	case "wss", "https":
		secure = true
	default:
		return nil, fmt.Errorf("websocket: unsupported scheme %q", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		if secure {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	netConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, fmt.Errorf("websocket: dial: %w", err)
	}
	// The upgrade write/read below block on the socket and would ignore a
	// context without a deadline; close the socket on ctx cancellation so a
	// caller can abort the handshake against a server that accepted TCP but
	// never answers. The watch stops once Dial returns, so cancelling the
	// dial context later never touches an established connection.
	stopCancelWatch := context.AfterFunc(ctx, func() { _ = netConn.Close() })
	defer stopCancelWatch()
	if secure {
		tlsConn := tls.Client(netConn, &tls.Config{ServerName: u.Hostname()})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = netConn.Close()
			return nil, fmt.Errorf("websocket: tls: %w", err)
		}
		netConn = tlsConn
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = netConn.SetDeadline(dl)
	}

	key, err := GenerateKey()
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := writeUpgrade(netConn, u, key, header); err != nil {
		_ = netConn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("websocket: handshake aborted: %w", ctxErr)
		}
		return nil, err
	}
	br := bufio.NewReader(netConn)
	if err := verifyUpgrade(br, key); err != nil {
		_ = netConn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("websocket: handshake aborted: %w", ctxErr)
		}
		return nil, err
	}
	_ = netConn.SetDeadline(time.Time{}) // clear the handshake deadline
	return &Conn{netConn: netConn, br: br, tls: secure}, nil
}

// writeUpgrade sends the HTTP/1.1 Upgrade request.
func writeUpgrade(w io.Writer, u *url.URL, key string, header http.Header) error {
	target := u.RequestURI()
	if target == "" {
		target = "/"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\n", target)
	fmt.Fprintf(&b, "Host: %s\r\n", u.Host)
	b.WriteString("Upgrade: websocket\r\n")
	b.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&b, "Sec-WebSocket-Key: %s\r\n", key)
	b.WriteString("Sec-WebSocket-Version: 13\r\n")
	for k, vs := range header {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// verifyUpgrade reads the 101 response and checks the accept key.
func verifyUpgrade(br *bufio.Reader, key string) error {
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		return fmt.Errorf("websocket: read handshake: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("websocket: handshake failed: %s", resp.Status)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		return fmt.Errorf("websocket: server did not upgrade")
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != AcceptKey(key) {
		return fmt.Errorf("websocket: bad Sec-WebSocket-Accept")
	}
	return nil
}

// writeTimeout bounds how long a single frame write may block on the socket.
// WriteMessage is called from the UI thread, so without a deadline a stalled
// server (a full TCP receive window that it never drains) would block the write
// — and freeze the whole app — indefinitely. A var so tests can shrink it.
var writeTimeout = 10 * time.Second

// writeFrameLocked writes one masked frame under writeTimeout so a stalled peer
// can't block the caller forever. A fresh deadline is armed before every write,
// so a stale deadline from a prior write never trips the next one. The caller
// must hold wmu. Setting a write deadline does not affect the concurrent reader
// (read and write deadlines are independent).
func (c *Conn) writeFrameLocked(f Frame) error {
	_ = c.netConn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return WriteFrame(c.netConn, f, true)
}

// WriteMessage sends a single-frame (FIN) message with the given opcode, masked
// per the client requirement.
func (c *Conn) WriteMessage(opcode byte, data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrameLocked(Frame{Fin: true, Opcode: opcode, Payload: data})
}

// maxMessageBytes caps a reassembled message's total size. Individual frames
// are already capped (maxFramePayload), but a stream of non-FIN fragments
// would otherwise grow the reassembly buffer without bound — the same hostile
// server the frame cap defends against. A var, not a const, so tests can
// shrink it instead of allocating real 64 MiB messages.
var maxMessageBytes = maxFramePayload

// ReadMessage returns the next text or binary message, reassembling continuation
// frames. Control frames are handled inline: a ping is answered with a pong, a
// pong is ignored, and a close ends the stream (a close is echoed) — surfaced as
// io.EOF. The returned opcode is OpText or OpBinary. Messages reassembling past
// maxMessageBytes are rejected with an error.
func (c *Conn) ReadMessage() (opcode byte, data []byte, err error) {
	var buf []byte
	msgOp := byte(0)
	for {
		f, err := ReadFrame(c.br)
		if err != nil {
			return 0, nil, err
		}
		switch {
		case f.Opcode == OpPing:
			if err := c.writeControl(OpPong, f.Payload); err != nil {
				return 0, nil, err
			}
		case f.Opcode == OpPong:
			// ignore
		case f.Opcode == OpClose:
			_ = c.writeControl(OpClose, f.Payload)
			return 0, nil, io.EOF
		case f.Opcode == OpContinuation, f.Opcode == OpText, f.Opcode == OpBinary:
			if f.Opcode != OpContinuation {
				msgOp = f.Opcode
			}
			if len(buf)+len(f.Payload) > maxMessageBytes {
				return 0, nil, fmt.Errorf("websocket: message exceeds %d bytes", maxMessageBytes)
			}
			buf = append(buf, f.Payload...)
			if f.Fin {
				return msgOp, buf, nil
			}
		default:
			return 0, nil, fmt.Errorf("websocket: unexpected opcode %#x", f.Opcode)
		}
	}
}

// writeControl sends a masked control frame (payload capped at 125 bytes by the
// spec; longer payloads are truncated rather than rejected since the only
// callers are pong/close echoes of server data).
func (c *Conn) writeControl(opcode byte, payload []byte) error {
	if len(payload) > 125 {
		payload = payload[:125]
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrameLocked(Frame{Fin: true, Opcode: opcode, Payload: payload})
}

// Close sends a close frame (best-effort) and closes the underlying connection.
// A blocked ReadMessage unblocks with an error once the connection is closed.
func (c *Conn) Close() error {
	_ = c.writeControl(OpClose, nil)
	return c.netConn.Close()
}
