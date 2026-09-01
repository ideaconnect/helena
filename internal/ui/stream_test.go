package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestStreamShowsFullTranscript: the coalesced repaint path must still deliver
// every event to the viewer — a queued repaint snapshots the newest
// accumulated text when it runs, so bursts collapsing into one paint lose
// nothing and the final event is always painted.
func TestStreamShowsFullTranscript(t *testing.T) {
	const events = 40
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for i := 0; i < events; i++ {
			fmt.Fprintf(w, "data: ev-%d\n\n", i)
			fl.Flush()
		}
	}))
	defer srv.Close()

	m := newAuthUI(t)
	origDone := streamWorkerDone
	workerDone := make(chan struct{})
	streamWorkerDone = func() { close(workerDone) }
	t.Cleanup(func() { streamWorkerDone = origDone })
	// Join on every exit path, not just the happy one: t.Fatal is a
	// runtime.Goexit, so the timeout branch below would otherwise abandon a
	// worker that is by definition still running, and under Fyne's test driver
	// fyne.Do runs inline on that worker — its final widget writes would land
	// in whatever test comes next. Cleanups run after the deferred srv.Close(),
	// so by the time this waits the server is gone and the worker unblocks.
	t.Cleanup(func() {
		select {
		case <-workerDone:
		case <-time.After(10 * time.Second):
			t.Error("stream worker did not finish")
		}
	})

	m.URL.SetText(srv.URL)
	m.streamSend()
	select {
	case <-workerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("stream worker did not finish")
	}

	var want strings.Builder
	for i := 0; i < events; i++ {
		fmt.Fprintf(&want, "ev-%d\n\n", i)
	}
	deadline := time.Now().Add(2 * time.Second)
	for string(m.pv.Source()) != want.String() {
		if time.Now().After(deadline) {
			t.Fatalf("viewer shows %d bytes, want %d (the full transcript)", len(m.pv.Source()), want.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
