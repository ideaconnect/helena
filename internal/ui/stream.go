package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/sse"
	"github.com/idct/helena/internal/vars"
)

// streamOrStop is the Stream button handler (#74): stop an open stream, else
// start one. A stream and a normal Send are mutually exclusive.
func (m *MainUI) streamOrStop() {
	if m.streamCancel != nil {
		m.streamCancel()
		return
	}
	m.streamSend()
}

// streamSend opens an SSE stream for the current request and appends events to
// the response Body view live (#74). It deliberately bypasses the chain/tab
// machinery — streaming is a focused, single-request mode. Snapshots are taken
// on the UI goroutine; the stream runs on a worker and marshals updates back via
// fyne.Do (the off-UI-goroutine invariant).
// streamWorkerDone is a test seam: the stream worker calls it as its very last
// act (after its final fyne.Do), so a test can wait for the goroutine to fully
// finish instead of leaking UI work into the next test (Fyne's caches are
// process-global, so a straggler races with later tests under -race).
var streamWorkerDone = func() {}

func (m *MainUI) streamSend() {
	if m.sendCancel != nil || m.streamCancel != nil {
		return // a Send or stream is already in flight
	}
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}

	var req model.Request
	if m.currentRequest != nil {
		m.syncBodyFromEditor()
		req = snapshotRequest(*m.currentRequest)
		req.Auth = m.sess.EffectiveAuth(m.currentRequestID)
		req.Variables = withFolderVars(m.sess.SnapshotAncestorVars(m.currentRequestID), req.Variables)
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected()), URL: m.URL.Text}
	}

	resolver := vars.New(
		m.sess.SnapshotGlobalVars(), m.sess.SnapshotActiveDotEnvVars(),
		m.sess.SnapshotActiveCollectionVars(), m.sess.SnapshotActiveEnvVars(),
		enabledRequestVars(req.Variables), m.sess.SnapshotEnvOverlay(),
	).WithFallback(vars.Dynamic)

	client := httpclient.NewWithTransport(m.sess.Settings(), m.sessionTransport())
	client.SetCookieJar(m.sess.CookieJar())

	ctx, cancel := context.WithCancel(context.Background())
	m.streamCancel = cancel
	m.setStreamStopButton()
	m.Status.SetText("Streaming…")
	// Blanking the shared viewer discards the previous response's model; reclaim
	// a large one. (The repaints in the stream loop below are a hot path and
	// deliberately do NOT reclaim — that would be a full GC per repaint.)
	freed := len(m.pv.Source())
	m.pv.SetData(nil, prettyview.FormatRaw)
	reclaimAfterLargeBody(freed)

	// Repaint coalescing: SetData re-parses the ENTIRE accumulated transcript,
	// so painting once per event is O(total²) over the stream's life and floods
	// the UI queue under bursty streams. Events append on the worker; at most
	// one repaint closure is queued at a time, and it snapshots the newest text
	// when it actually runs on the UI goroutine (latest-wins, so the final
	// event is always painted).
	var (
		repaintMu     sync.Mutex
		repaintText   string
		repaintEvents int
		repaintQueued bool
	)
	repaint := func() {
		repaintMu.Lock()
		text, n := repaintText, repaintEvents
		repaintQueued = false
		repaintMu.Unlock()
		m.pv.SetData([]byte(text), prettyview.FormatRaw)
		m.Status.SetText(fmt.Sprintf("Streaming… %d event(s)%s", n, streamTruncNote(len(text))))
	}

	go func() {
		defer streamWorkerDone() // runs after the final fyne.Do below completes
		defer cancel()
		var buf strings.Builder
		count := 0
		err := client.Stream(ctx, req, resolver,
			func(meta httpclient.StreamMeta) {
				status := meta.Status
				fyne.Do(func() { m.Status.SetText("Streaming (" + status + ")…") })
			},
			func(ev sse.Event) bool {
				// buf is worker-only; Builder.String() aliases the buffer
				// without copying, and appends never mutate returned strings.
				// The transcript stops growing at the viewer's display cap:
				// unlike a Send there is no tab cache behind it, so bytes past
				// the cap would be both invisible AND unsaveable — accumulating
				// them only grows memory. Events keep counting either way.
				if buf.Len() < displayBodyCap {
					buf.WriteString(formatSSEEvent(ev))
				}
				count++
				repaintMu.Lock()
				repaintText, repaintEvents = buf.String(), count
				queue := !repaintQueued
				repaintQueued = true
				repaintMu.Unlock()
				if queue {
					fyne.Do(repaint)
				}
				return true
			},
		)
		final, finalLen := count, buf.Len()
		canceled := ctx.Err() == context.Canceled
		fyne.Do(func() {
			m.resetStreamButton()
			switch {
			case canceled:
				m.Status.SetText(fmt.Sprintf("Stream stopped (%d event(s))%s", final, streamTruncNote(finalLen)))
			case err != nil:
				m.Status.SetText("Stream ended: " + err.Error())
			default:
				m.Status.SetText(fmt.Sprintf("Stream ended (%d event(s))%s", final, streamTruncNote(finalLen)))
			}
		})
	}()
}

// streamTruncNote returns the status-line suffix flagging that the stream
// transcript hit the display cap (further event payloads were dropped, not
// just hidden — the stream path has no tab cache to save them from).
func streamTruncNote(transcriptLen int) string {
	if transcriptLen < displayBodyCap {
		return ""
	}
	return fmt.Sprintf(" · transcript capped at %d MiB", displayBodyCap>>20)
}

// formatSSEEvent renders one event for the live Body view: the optional
// event-type and id headers followed by the data payload and a blank line.
func formatSSEEvent(ev sse.Event) string {
	var b strings.Builder
	if ev.Type != "" && ev.Type != "message" {
		b.WriteString("event: " + ev.Type + "\n")
	}
	if ev.ID != "" {
		b.WriteString("id: " + ev.ID + "\n")
	}
	b.WriteString(ev.Data)
	b.WriteString("\n\n")
	return b.String()
}

// setStreamStopButton flips the Stream button to its Stop state.
func (m *MainUI) setStreamStopButton() {
	m.Stream.SetIcon(themedIcon("circle-stop"))
	m.Stream.Importance = widget.WarningImportance
	m.Stream.Refresh()
}

// resetStreamButton restores the Stream button to its idle state and clears the
// in-flight cancel func.
func (m *MainUI) resetStreamButton() {
	m.streamCancel = nil
	m.Stream.SetIcon(themedIcon("play"))
	m.Stream.Importance = widget.MediumImportance
	m.Stream.Refresh()
}
