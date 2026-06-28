package ui

import (
	"testing"

	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/sse"
)

// TestFormatSSEEvent covers the live-view rendering of an event: type + id
// headers (omitted when default/empty) plus the data payload and a blank line.
func TestFormatSSEEvent(t *testing.T) {
	got := formatSSEEvent(sse.Event{Type: "tick", ID: "3", Data: "hello"})
	if got != "event: tick\nid: 3\nhello\n\n" {
		t.Errorf("formatSSEEvent = %q", got)
	}
	// Default type + empty id are omitted.
	if got := formatSSEEvent(sse.Event{Type: "message", Data: "x"}); got != "x\n\n" {
		t.Errorf("default-type formatSSEEvent = %q", got)
	}
}

// TestStreamButtonToggle verifies the Stream button flips to a warning Stop
// state and back, clearing the cancel func on reset.
func TestStreamButtonToggle(t *testing.T) {
	m := newAuthUI(t)
	m.streamCancel = func() {}
	m.setStreamStopButton()
	if m.Stream.Importance != widget.WarningImportance {
		t.Errorf("stop-state importance = %v, want Warning", m.Stream.Importance)
	}
	m.resetStreamButton()
	if m.streamCancel != nil {
		t.Error("resetStreamButton should clear streamCancel")
	}
	if m.Stream.Importance != widget.MediumImportance {
		t.Errorf("idle importance = %v, want Medium", m.Stream.Importance)
	}
}

// TestStreamOrStopCancels verifies that while a stream is open the button taps
// cancel (Stop) rather than starting a new stream.
func TestStreamOrStopCancels(t *testing.T) {
	m := newAuthUI(t)
	canceled := false
	m.streamCancel = func() { canceled = true }
	m.streamOrStop()
	if !canceled {
		t.Error("streamOrStop with an open stream should cancel it")
	}
}

// TestStreamSendEmptyURL verifies the no-URL guard: no stream is started.
func TestStreamSendEmptyURL(t *testing.T) {
	m := newAuthUI(t)
	m.URL.SetText("")
	m.streamSend()
	if m.streamCancel != nil {
		t.Error("streamSend with an empty URL must not start a stream")
	}
}
