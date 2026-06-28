package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestIsWebSocketURL covers the ws/wss scheme detection used to branch Send.
func TestIsWebSocketURL(t *testing.T) {
	cases := map[string]bool{
		"ws://example.com/s":  true,
		"wss://example.com":   true,
		"  WS://upper":        true,
		"http://example.com":  false,
		"https://example.com": false,
		"":                    false,
		"example.com":         false,
	}
	for in, want := range cases {
		if got := isWebSocketURL(in); got != want {
			t.Errorf("isWebSocketURL(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestWSTranscriptLine covers the directional formatting of a transcript entry.
func TestWSTranscriptLine(t *testing.T) {
	if got := wsTranscriptLine(true, "hello"); got != "→ hello" {
		t.Errorf("sent = %q", got)
	}
	if got := wsTranscriptLine(false, "world"); got != "← world" {
		t.Errorf("received = %q", got)
	}
}

// TestCurrentRequestHeaders verifies the active request's enabled headers are
// resolved into an http.Header for the WebSocket upgrade (disabled rows dropped,
// {{vars}} substituted).
func TestCurrentRequestHeaders(t *testing.T) {
	m := newAuthUI(t)
	if h := m.currentRequestHeaders(vars.New(nil)); h != nil {
		t.Errorf("no request → nil headers, got %v", h)
	}
	m.currentRequest = &model.Request{Headers: []model.KeyValue{
		{Enabled: true, Key: "Authorization", Value: "Bearer {{tok}}"},
		{Enabled: false, Key: "X-Skip", Value: "no"},
	}}
	res := vars.New(map[string]string{"tok": "abc"})
	h := m.currentRequestHeaders(res)
	if got := h["Authorization"]; len(got) != 1 || got[0] != "Bearer abc" {
		t.Errorf("Authorization = %v, want [Bearer abc]", got)
	}
	if _, ok := h["X-Skip"]; ok {
		t.Error("disabled header should be dropped")
	}
}

// TestCurrentRequestVars verifies the active request's enabled variables become
// a resolver scope (nil when no request is open).
func TestCurrentRequestVars(t *testing.T) {
	m := newAuthUI(t)
	if m.currentRequestVars() != nil {
		t.Error("no request → nil vars")
	}
	m.currentRequest = &model.Request{Variables: []model.Variable{{Enabled: true, Key: "a", Value: "1"}}}
	if v := m.currentRequestVars(); v["a"] != "1" {
		t.Errorf("vars = %v, want a=1", v)
	}
}
