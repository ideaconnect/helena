package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/sse"
	"github.com/idct/helena/internal/vars"
)

// TestStreamDeliversEvents drives Stream against a server that emits three SSE
// events then closes, and verifies they arrive in order with metadata (#74).
func TestStreamDeliversEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "id: %d\nevent: tick\ndata: msg-%d\n\n", i, i)
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	c := New(model.DefaultSettings())
	var opened bool
	var got []sse.Event
	err := c.Stream(context.Background(),
		model.Request{Method: model.GET, URL: srv.URL + "/events"},
		vars.New(nil),
		func(StreamMeta) { opened = true },
		func(ev sse.Event) bool { got = append(got, ev); return true },
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !opened {
		t.Error("onOpen was not called")
	}
	if len(got) != 3 || got[0].Data != "msg-1" || got[2].Type != "tick" || got[2].ID != "3" {
		t.Fatalf("events = %+v", got)
	}
}

// TestStreamStopsOnFalse verifies returning false from onEvent stops streaming.
func TestStreamStopsOnFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	c := New(model.DefaultSettings())
	n := 0
	err := c.Stream(context.Background(),
		model.Request{Method: model.GET, URL: srv.URL},
		vars.New(nil), nil,
		func(sse.Event) bool { n++; return n < 2 }, // stop after the 2nd
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if n != 2 {
		t.Errorf("received %d events, want 2 (stopped early)", n)
	}
}

// TestStreamNon2xxErrors verifies a non-2xx response is an error, not a stream.
func TestStreamNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(model.DefaultSettings())
	err := c.Stream(context.Background(), model.Request{Method: model.GET, URL: srv.URL},
		vars.New(nil), nil, func(sse.Event) bool { return true })
	if err == nil {
		t.Error("expected an error for a 404 stream")
	}
}

// TestStreamContextCancel verifies cancelling ctx stops a long-lived stream.
func TestStreamContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 1000; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(2 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := New(model.DefaultSettings())
	ctx, cancel := context.WithCancel(context.Background())
	got := 0
	err := c.Stream(ctx, model.Request{Method: model.GET, URL: srv.URL}, vars.New(nil), nil,
		func(sse.Event) bool {
			got++
			if got == 2 {
				cancel()
			}
			return true
		})
	if err == nil {
		t.Error("expected ctx cancellation error")
	}
}

// TestStreamOutlivesClientTimeout: an open stream must NOT be killed by the
// client-wide TimeoutSeconds — that deadline covers the body read, and for
// SSE the body read is the stream itself. Events keep arriving past the
// timeout and the stream ends cleanly when the server closes.
func TestStreamOutlivesClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: first\n\n")
		fl.Flush()
		time.Sleep(1500 * time.Millisecond) // past the 1 s client timeout
		_, _ = io.WriteString(w, "data: second\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	c := New(model.Settings{TimeoutSeconds: 1})
	var got []string
	err := c.Stream(context.Background(),
		model.Request{Method: model.GET, URL: srv.URL}, vars.New(), nil,
		func(ev sse.Event) bool { got = append(got, ev.Data); return true })
	if err != nil {
		t.Fatalf("Stream: %v (the client timeout must not kill an open stream)", err)
	}
	if len(got) != 2 || got[1] != "second" {
		t.Fatalf("events = %q, want [first second] across the timeout boundary", got)
	}
}

// TestStreamHeaderTimeoutStillBounds: TimeoutSeconds still bounds the
// connect + response-header phase, so a server that accepts and stalls
// fails fast instead of hanging the stream forever.
func TestStreamHeaderTimeoutStillBounds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never write headers; unblocks when the client gives up
	}))
	defer srv.Close()

	c := New(model.Settings{TimeoutSeconds: 1})
	start := time.Now()
	err := c.Stream(context.Background(),
		model.Request{Method: model.GET, URL: srv.URL}, vars.New(), nil,
		func(sse.Event) bool { return true })
	if err == nil {
		t.Fatal("expected a header-phase timeout error")
	}
	if !strings.Contains(err.Error(), "no response within") {
		t.Errorf("err = %v, want the header-timeout message", err)
	}
	if e := time.Since(start); e > 5*time.Second {
		t.Errorf("timed out after %v, want ~1 s", e)
	}
}
