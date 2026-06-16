package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// countingServer starts a test server that counts newly-opened TCP connections
// via ConnState, so a test can tell connection reuse from re-dialing.
func countingServer(t *testing.T) (url string, newConns *int32, closeFn func()) {
	t.Helper()
	var n int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			atomic.AddInt32(&n, 1)
		}
	}
	srv.Start()
	return srv.URL, &n, srv.Close
}

// TestSharedTransportReusesConnections verifies two sequential sends built with
// NewWithTransport over the same transport reuse the connection pool — only one
// TCP connection is opened — whereas fresh per-call transports re-dial (#52).
func TestSharedTransportReusesConnections(t *testing.T) {
	url, newConns, closeFn := countingServer(t)
	defer closeFn()
	req := model.Request{Method: model.GET, URL: url}

	tr := NewTransport(model.DefaultSettings())
	for i := 0; i < 3; i++ {
		c := NewWithTransport(model.DefaultSettings(), tr)
		if _, err := c.Do(context.Background(), req, vars.New()); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(newConns); got != 1 {
		t.Errorf("shared transport opened %d connections, want 1 (pool not reused)", got)
	}

	// Control: a fresh transport per call re-dials every time.
	url2, newConns2, closeFn2 := countingServer(t)
	defer closeFn2()
	req2 := model.Request{Method: model.GET, URL: url2}
	for i := 0; i < 3; i++ {
		c := New(model.DefaultSettings())
		if _, err := c.Do(context.Background(), req2, vars.New()); err != nil {
			t.Fatalf("control send %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(newConns2); got < 2 {
		t.Errorf("fresh-transport control opened %d connections, want >=2", got)
	}
}

// TestNewWithTransportHonorsSettings verifies the per-call Client still applies
// timeout/redirect policy even when reusing a shared transport.
func TestNewWithTransportHonorsSettings(t *testing.T) {
	tr := NewTransport(model.DefaultSettings())
	c := NewWithTransport(model.Settings{TimeoutSeconds: 7}, tr)
	if c.http.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s", c.http.Timeout)
	}
	if c.http.Transport != tr {
		t.Error("client did not reuse the supplied transport")
	}
	// A nil transport falls back to a fresh one rather than panicking.
	if got := NewWithTransport(model.DefaultSettings(), nil); got.http.Transport == nil {
		t.Error("nil transport was not replaced with a fresh one")
	}
}
