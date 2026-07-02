package ui

import (
	"context"
	"fmt"
	"strings"

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
	// a large one. (The per-event SetData in the stream loop below is a hot path
	// and deliberately does NOT reclaim — that would be a full GC per event.)
	freed := len(m.pv.Source())
	m.pv.SetData(nil, prettyview.FormatRaw)
	reclaimAfterLargeBody(freed)

	go func() {
		defer cancel()
		var buf strings.Builder
		count := 0
		err := client.Stream(ctx, req, resolver,
			func(meta httpclient.StreamMeta) {
				status := meta.Status
				fyne.Do(func() { m.Status.SetText("Streaming (" + status + ")…") })
			},
			func(ev sse.Event) bool {
				buf.WriteString(formatSSEEvent(ev))
				count++
				text, n := buf.String(), count
				fyne.Do(func() {
					m.pv.SetData([]byte(text), prettyview.FormatRaw)
					m.Status.SetText(fmt.Sprintf("Streaming… %d event(s)", n))
				})
				return true
			},
		)
		final := count
		canceled := ctx.Err() == context.Canceled
		fyne.Do(func() {
			m.resetStreamButton()
			switch {
			case canceled:
				m.Status.SetText(fmt.Sprintf("Stream stopped (%d event(s))", final))
			case err != nil:
				m.Status.SetText("Stream ended: " + err.Error())
			default:
				m.Status.SetText(fmt.Sprintf("Stream ended (%d event(s))", final))
			}
		})
	}()
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
