package ui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
	"github.com/idct/helena/internal/websocket"
)

// isWebSocketURL reports whether raw (a possibly-templated URL) targets a
// WebSocket endpoint — used to branch Send into the WebSocket session (#72).
func isWebSocketURL(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(s, "ws://") || strings.HasPrefix(s, "wss://")
}

// wsTranscriptLine formats one transcript entry: an arrow direction marker plus
// the message text.
func wsTranscriptLine(sent bool, msg string) string {
	if sent {
		return "→ " + msg
	}
	return "← " + msg
}

// openWebSocket opens a modeless WebSocket session dialog (#72): it dials the
// resolved ws:// URL, streams received messages into a transcript, and lets the
// user send text messages. The connection runs on a worker goroutine; UI
// updates marshal back via fyne.Do. Closing the dialog closes the connection
// (which unblocks the read goroutine) — or, when the dial is still in flight,
// cancels it, so an unresponsive server can't leak the worker and its socket.
func (m *MainUI) openWebSocket() {
	if m.win == nil {
		return
	}
	raw := strings.TrimSpace(m.URL.Text)
	resolver := vars.New(
		m.sess.SnapshotGlobalVars(), m.sess.SnapshotActiveDotEnvVars(),
		m.sess.SnapshotActiveCollectionVars(), m.sess.SnapshotActiveEnvVars(),
		m.currentRequestVars(), m.sess.SnapshotEnvOverlay(),
	).WithFallback(vars.Dynamic)
	url, _ := resolver.Resolve(raw)
	if !isWebSocketURL(url) {
		m.Status.SetText("WebSocket needs a ws:// or wss:// URL")
		return
	}

	transcript := widget.NewMultiLineEntry()
	transcript.Wrapping = fyne.TextWrapWord
	transcript.Disable()
	input := widget.NewEntry()
	input.SetPlaceHolder("Type a message and press Send")

	// The dial runs under a cancelable context so closing the dialog while the
	// handshake is still in flight aborts it instead of leaking the goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	var conn *websocket.Conn // only ever read/written on the UI goroutine
	dialogClosed := false    // UI goroutine only; gates the publish below
	appendLine := func(s string) {
		fyne.Do(func() {
			if transcript.Text == "" {
				transcript.SetText(s)
			} else {
				transcript.SetText(transcript.Text + "\n" + s)
			}
			transcript.CursorRow = strings.Count(transcript.Text, "\n")
		})
	}
	send := func() {
		if conn == nil || strings.TrimSpace(input.Text) == "" {
			return
		}
		msg := input.Text
		if err := conn.WriteMessage(websocket.OpText, []byte(msg)); err != nil {
			appendLine("✗ send failed: " + err.Error())
			return
		}
		appendLine(wsTranscriptLine(true, msg))
		input.SetText("")
	}
	input.OnSubmitted = func(string) { send() }
	sendBtn := widget.NewButton("Send", send)

	bottom := container.NewBorder(nil, nil, nil, sendBtn, input)
	content := container.NewBorder(nil, bottom, nil, nil, container.NewVScroll(transcript))
	d := dialog.NewCustom("WebSocket — "+url, "Close", content, m.win)
	d.Resize(fyne.NewSize(560, 420))
	d.SetOnClosed(func() {
		dialogClosed = true
		cancel() // aborts a dial/handshake still in flight
		if conn != nil {
			_ = conn.Close()
		}
	})
	d.Show()

	go func() {
		c, err := websocket.Dial(ctx, url, m.currentRequestHeaders(resolver))
		if err != nil {
			appendLine("✗ connect failed: " + err.Error())
			return
		}
		// Publish on the UI goroutine — unless the dialog was closed while the
		// dial was in flight, in which case OnClosed saw conn==nil and nothing
		// else owns c, so it must be closed right here.
		published := false
		fyne.DoAndWait(func() {
			if dialogClosed {
				return
			}
			conn = c
			published = true
		})
		if !published {
			_ = c.Close()
			return
		}
		appendLine("✓ connected")
		for {
			op, data, err := c.ReadMessage()
			if err != nil {
				appendLine("✗ closed: " + err.Error())
				return
			}
			if op == websocket.OpText {
				appendLine(wsTranscriptLine(false, string(data)))
			} else {
				appendLine(wsTranscriptLine(false, fmt.Sprintf("<%d binary bytes>", len(data))))
			}
		}
	}()
}

// currentRequestVars returns the active request's enabled variables as a
// resolver scope (empty when no request is open).
func (m *MainUI) currentRequestVars() map[string]string {
	if m.currentRequest == nil {
		return nil
	}
	return enabledRequestVars(m.currentRequest.Variables)
}

// currentRequestHeaders resolves the active request's enabled headers into an
// http.Header for the WebSocket upgrade (so an Authorization header carries into
// the handshake). Nil when no request is open.
func (m *MainUI) currentRequestHeaders(resolver *vars.Resolver) map[string][]string {
	if m.currentRequest == nil {
		return nil
	}
	h := map[string][]string{}
	for _, kv := range model.EnabledPairs(m.currentRequest.Headers) {
		name, _ := resolver.Resolve(kv.Key)
		value, _ := resolver.Resolve(kv.Value)
		if name != "" {
			h[name] = append(h[name], value)
		}
	}
	if len(h) == 0 {
		return nil
	}
	return h
}
