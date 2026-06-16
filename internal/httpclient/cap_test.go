package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestMaxResponseBytesSettingControlsTruncation verifies the configurable body
// cap (#111): a low setting truncates a body that a high setting reads in full.
func TestMaxResponseBytesSettingControlsTruncation(t *testing.T) {
	body := strings.Repeat("x", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	req := model.Request{Method: model.GET, URL: srv.URL}

	low := New(model.Settings{MaxResponseBytes: 1000})
	resp, err := low.Do(context.Background(), req, vars.New())
	if err != nil {
		t.Fatalf("low.Do: %v", err)
	}
	if !resp.Truncated || len(resp.Body) != 1000 {
		t.Errorf("low cap: truncated=%v len=%d, want truncated len=1000", resp.Truncated, len(resp.Body))
	}

	high := New(model.Settings{MaxResponseBytes: 1 << 20})
	resp, err = high.Do(context.Background(), req, vars.New())
	if err != nil {
		t.Fatalf("high.Do: %v", err)
	}
	if resp.Truncated || len(resp.Body) != 5000 {
		t.Errorf("high cap: truncated=%v len=%d, want full len=5000", resp.Truncated, len(resp.Body))
	}
}

// TestMaxResponseBytesFallsBackToDefault verifies an unset (<=0) setting yields
// the built-in default cap rather than an unbounded (or zero) read.
func TestMaxResponseBytesFallsBackToDefault(t *testing.T) {
	for _, v := range []int64{0, -1} {
		c := New(model.Settings{MaxResponseBytes: v})
		if got := c.maxResponseBytes(); got != model.DefaultMaxResponseBytes {
			t.Errorf("MaxResponseBytes=%d: effective cap = %d, want default %d", v, got, model.DefaultMaxResponseBytes)
		}
	}
	c := New(model.Settings{MaxResponseBytes: 4096})
	if got := c.maxResponseBytes(); got != 4096 {
		t.Errorf("effective cap = %d, want 4096", got)
	}
}
