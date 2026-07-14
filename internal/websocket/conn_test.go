package websocket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// handshakeServer completes the WebSocket handshake then hands the hijacked
// ReadWriter to after, so a test can drive any frame sequence as the server.
func handshakeServer(t *testing.T, after func(brw *bufio.ReadWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", AcceptKey(r.Header.Get("Sec-WebSocket-Key")))
		_ = brw.Flush()
		after(brw)
		time.Sleep(10 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialMsg dials the server, reads one message, and returns it.
func dialMsg(t *testing.T, url string) (byte, []byte, error) {
	t.Helper()
	c, err := Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	return c.ReadMessage()
}

// echoServer is an httptest handler that completes the WebSocket handshake by
// hijacking the connection, then echoes text/binary frames and answers pings —
// using the package's own (RFC-verified) codec as the server peer.
func echoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter is not a Hijacker")
			return
		}
		conn, brw, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		accept := AcceptKey(r.Header.Get("Sec-WebSocket-Key"))
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
		_ = brw.Flush()
		for {
			f, err := ReadFrame(brw)
			if err != nil {
				return
			}
			switch f.Opcode {
			case OpClose:
				_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpClose}, false)
				_ = brw.Flush()
				return
			case OpPing:
				_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpPong, Payload: f.Payload}, false)
				_ = brw.Flush()
			case OpText, OpBinary:
				// Echo unmasked (server→client frames must not be masked).
				_ = WriteFrame(brw, Frame{Fin: true, Opcode: f.Opcode, Payload: f.Payload}, false)
				_ = brw.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// wsURL converts an http:// httptest URL to ws://.
func wsURL(s string) string { return "ws" + strings.TrimPrefix(s, "http") }

// TestDialAndEcho drives the full handshake + a masked send + an unmasked echo
// back through Dial / WriteMessage / ReadMessage (#72).
func TestDialAndEcho(t *testing.T) {
	srv := echoServer(t)
	c, err := Dial(context.Background(), wsURL(srv.URL)+"/chat", nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.WriteMessage(OpText, []byte("ping-pong")); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	op, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if op != OpText || string(data) != "ping-pong" {
		t.Errorf("echo = op %#x %q", op, data)
	}

	// A second message confirms the connection stays usable.
	_ = c.WriteMessage(OpText, []byte("again"))
	if _, data, _ := c.ReadMessage(); string(data) != "again" {
		t.Errorf("second echo = %q", data)
	}
}

// TestDialBadScheme rejects a non-ws scheme without dialing.
func TestDialBadScheme(t *testing.T) {
	if _, err := Dial(context.Background(), "ftp://example.com", nil); err == nil {
		t.Error("expected an error for a non-ws scheme")
	}
}

// TestDialHandshakeRejected verifies a server that doesn't return 101 fails the
// handshake.
func TestDialHandshakeRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // not a 101
	}))
	defer srv.Close()
	if _, err := Dial(context.Background(), wsURL(srv.URL), nil); err == nil {
		t.Error("expected handshake failure on a non-101 response")
	}
}

// TestReadMessageHandlesPing verifies a server-initiated ping is answered with a
// pong while ReadMessage continues to the real message.
func TestReadMessageHandlesPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj := w.(http.Hijacker)
		conn, brw, _ := hj.Hijack()
		defer conn.Close()
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", AcceptKey(r.Header.Get("Sec-WebSocket-Key")))
		_ = brw.Flush()
		// Send a ping, then a text message.
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpPing, Payload: []byte("p")}, false)
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpText, Payload: []byte("after-ping")}, false)
		_ = brw.Flush()
		// Expect a pong back from the client.
		if f, err := ReadFrame(brw); err != nil || f.Opcode != OpPong {
			t.Errorf("expected a pong, got %+v err %v", f, err)
		}
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()

	c, err := Dial(context.Background(), wsURL(srv.URL), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	op, data, err := c.ReadMessage()
	if err != nil || op != OpText || string(data) != "after-ping" {
		t.Errorf("ReadMessage after ping = op %#x %q err %v", op, data, err)
	}
}

// TestReadMessageFragmented verifies continuation-frame reassembly.
func TestReadMessageFragmented(t *testing.T) {
	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		_ = WriteFrame(brw, Frame{Fin: false, Opcode: OpText, Payload: []byte("frag")}, false)
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpContinuation, Payload: []byte("ment")}, false)
		_ = brw.Flush()
	})
	if op, data, err := dialMsg(t, wsURL(srv.URL)); err != nil || op != OpText || string(data) != "fragment" {
		t.Errorf("fragmented = op %#x %q err %v", op, data, err)
	}
}

// TestReadMessageClose verifies a server close frame surfaces as io.EOF.
func TestReadMessageClose(t *testing.T) {
	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpClose}, false)
		_ = brw.Flush()
	})
	if _, _, err := dialMsg(t, wsURL(srv.URL)); err != io.EOF {
		t.Errorf("close should surface as io.EOF, got %v", err)
	}
}

// TestReadMessagePongThenText verifies an unsolicited pong is ignored.
func TestReadMessagePongThenText(t *testing.T) {
	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpPong}, false)
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpText, Payload: []byte("hi")}, false)
		_ = brw.Flush()
	})
	if _, data, err := dialMsg(t, wsURL(srv.URL)); err != nil || string(data) != "hi" {
		t.Errorf("pong-then-text = %q err %v", data, err)
	}
}

// TestReadMessageUnexpectedOpcode verifies a reserved opcode is an error.
func TestReadMessageUnexpectedOpcode(t *testing.T) {
	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: 0x3, Payload: []byte("?")}, false)
		_ = brw.Flush()
	})
	if _, _, err := dialMsg(t, wsURL(srv.URL)); err == nil {
		t.Error("expected an error for a reserved opcode")
	}
}

// TestReadMessageBigPing exercises the pong echo of an over-125-byte ping
// (truncated by writeControl) followed by the real message.
func TestReadMessageBigPing(t *testing.T) {
	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		big := make([]byte, 130)
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpPing, Payload: big}, false)
		_ = WriteFrame(brw, Frame{Fin: true, Opcode: OpText, Payload: []byte("ok")}, false)
		_ = brw.Flush()
		_, _ = ReadFrame(brw) // drain the client's pong
	})
	if _, data, err := dialMsg(t, wsURL(srv.URL)); err != nil || string(data) != "ok" {
		t.Errorf("big-ping = %q err %v", data, err)
	}
}

// TestDialSendsHeaders verifies extra request headers reach the server.
func TestDialSendsHeaders(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Token")
		conn, brw, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", AcceptKey(r.Header.Get("Sec-WebSocket-Key")))
		_ = brw.Flush()
		time.Sleep(10 * time.Millisecond)
	}))
	defer srv.Close()
	c, err := Dial(context.Background(), wsURL(srv.URL), http.Header{"X-Token": {"secret"}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if v := <-got; v != "secret" {
		t.Errorf("server saw X-Token = %q, want secret", v)
	}
}

// TestVerifyUpgradeBadAccept rejects a 101 with the wrong accept key.
func TestVerifyUpgradeBadAccept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		fmt.Fprint(brw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: wrong\r\n\r\n")
		_ = brw.Flush()
		time.Sleep(10 * time.Millisecond)
	}))
	defer srv.Close()
	if _, err := Dial(context.Background(), wsURL(srv.URL), nil); err == nil {
		t.Error("expected a bad-accept error")
	}
}

// TestVerifyUpgradeNoUpgradeHeader rejects a 101 missing Upgrade: websocket.
func TestVerifyUpgradeNoUpgradeHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		fmt.Fprintf(brw, "HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: %s\r\n\r\n", AcceptKey(r.Header.Get("Sec-WebSocket-Key")))
		_ = brw.Flush()
		time.Sleep(10 * time.Millisecond)
	}))
	defer srv.Close()
	if _, err := Dial(context.Background(), wsURL(srv.URL), nil); err == nil {
		t.Error("expected a missing-upgrade error")
	}
}

// TestDialBadURL rejects an unparseable URL.
func TestDialBadURL(t *testing.T) {
	if _, err := Dial(context.Background(), "://bad", nil); err == nil {
		t.Error("expected a parse error")
	}
}

// TestDialDefaultPortWS covers the ws:// default-port (:80) branch; the dial
// itself is expected to fail (nothing is a WebSocket server on :80 here).
func TestDialDefaultPortWS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Dial(ctx, "ws://127.0.0.1", nil); err == nil {
		t.Error("expected a dial/handshake error on :80")
	}
}

// TestDialTLSHandshakeFails covers the wss:// TLS branch: the httptest TLS
// server's cert isn't trusted by the default roots, so the TLS handshake fails.
func TestDialTLSHandshakeFails(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	wss := "wss" + strings.TrimPrefix(srv.URL, "https")
	if _, err := Dial(context.Background(), wss, nil); err == nil {
		t.Error("expected a TLS handshake error for an untrusted cert")
	}
}

// TestDialCancelAbortsHandshake: a server that accepts TCP but never answers
// the upgrade must not block Dial forever — cancelling the context aborts the
// in-flight handshake (the UI cancels when its dialog is closed mid-dial).
func TestDialCancelAbortsHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Consume the upgrade request but never reply; unblocks when the
		// aborted client closes its side.
		_, _ = io.Copy(io.Discard, conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, dialErr := Dial(ctx, "ws://"+ln.Addr().String(), nil)
		errCh <- dialErr
	}()
	time.Sleep(50 * time.Millisecond) // let the dial reach the handshake read
	cancel()
	select {
	case dialErr := <-errCh:
		if dialErr == nil {
			t.Fatal("Dial succeeded against a server that never answered the upgrade")
		}
		if !errors.Is(dialErr, context.Canceled) {
			t.Errorf("Dial error = %v, want context.Canceled in its chain", dialErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Dial did not abort within 2s of cancellation")
	}
}

// TestReadMessageCapsReassembledSize: fragmented frames that never FIN must
// not grow the reassembly buffer without bound — the per-frame cap alone does
// not stop a stream of under-cap fragments.
func TestReadMessageCapsReassembledSize(t *testing.T) {
	origMax := maxMessageBytes
	maxMessageBytes = 150
	t.Cleanup(func() { maxMessageBytes = origMax })

	srv := handshakeServer(t, func(brw *bufio.ReadWriter) {
		payload := []byte(strings.Repeat("a", 100))
		_ = WriteFrame(brw, Frame{Fin: false, Opcode: OpText, Payload: payload}, false)
		_ = WriteFrame(brw, Frame{Fin: false, Opcode: OpContinuation, Payload: payload}, false)
		_ = brw.Flush()
	})

	_, _, err := dialMsg(t, "ws"+strings.TrimPrefix(srv.URL, "http"))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ReadMessage err = %v, want the message-size cap error", err)
	}
}

// TestWriteMessageTimesOutOnStalledPeer pins the write deadline: WriteMessage is
// called from the UI thread, so a peer that never drains its receive window must
// not block the write forever (which would freeze the app). net.Pipe is fully
// synchronous — a write blocks until the other end reads — so leaving c2 unread
// makes the write hang; the deadline must cut it off with an error.
func TestWriteMessageTimesOutOnStalledPeer(t *testing.T) {
	old := writeTimeout
	writeTimeout = 50 * time.Millisecond
	defer func() { writeTimeout = old }()

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close() // c2 is never read from — the write has nowhere to go

	conn := &Conn{netConn: c1}
	done := make(chan error, 1)
	go func() { done <- conn.WriteMessage(OpText, []byte("hello")) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a write-deadline error on a stalled peer, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WriteMessage blocked past the write deadline — the UI thread would freeze")
	}
}

// TestWriteMessageSucceedsWhenPeerReads is the happy-path counterpart: with a
// peer draining the socket, the deadline never trips and the frame goes out.
func TestWriteMessageSucceedsWhenPeerReads(t *testing.T) {
	old := writeTimeout
	writeTimeout = 2 * time.Second
	defer func() { writeTimeout = old }()

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	// Drain c2 so the write completes.
	go func() { _, _ = ReadFrame(bufio.NewReader(c2)) }()

	conn := &Conn{netConn: c1}
	if err := conn.WriteMessage(OpText, []byte("hi")); err != nil {
		t.Fatalf("WriteMessage on a draining peer = %v, want nil", err)
	}
}
