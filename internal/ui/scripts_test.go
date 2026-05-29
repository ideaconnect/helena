package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestScriptsTabLoadAndWriteBack verifies that loading a request
// populates the two editors and that typing into either editor writes
// back into the in-memory request via OnChanged.
func TestScriptsTabLoadAndWriteBack(t *testing.T) {
	test.NewApp()

	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "Scripted", Method: model.GET, URL: "https://x/",
		Scripts: model.Scripts{
			PreRequest:   "console.log('pre');",
			PostResponse: "helena.env.set('T', response.json.token);",
		},
	}
	m.loadRequest(req, "0/r0")

	if m.preScriptEditor.Text != req.Scripts.PreRequest {
		t.Errorf("preScriptEditor = %q, want %q", m.preScriptEditor.Text, req.Scripts.PreRequest)
	}
	if m.postScriptEditor.Text != req.Scripts.PostResponse {
		t.Errorf("postScriptEditor = %q, want %q", m.postScriptEditor.Text, req.Scripts.PostResponse)
	}

	// Simulate the user typing into both editors.
	m.preScriptEditor.OnChanged("new pre")
	m.postScriptEditor.OnChanged("new post")
	if m.currentRequest.Scripts.PreRequest != "new pre" {
		t.Errorf("PreRequest = %q, want 'new pre'", m.currentRequest.Scripts.PreRequest)
	}
	if m.currentRequest.Scripts.PostResponse != "new post" {
		t.Errorf("PostResponse = %q, want 'new post'", m.currentRequest.Scripts.PostResponse)
	}
}

// TestScriptsTabSuppressedDuringLoad verifies that the OnChanged
// callbacks fired by loadRequest's programmatic SetText don't overwrite
// the model — same m.loading invariant every other tab respects.
func TestScriptsTabSuppressedDuringLoad(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "First", Method: model.GET, URL: "https://x/",
		Scripts: model.Scripts{PreRequest: "original"},
	}
	m.loadRequest(req, "0/r0")

	// Simulate switching to a different request whose pre-script is empty.
	other := &model.Request{Name: "Second", Method: model.GET, URL: "https://y/"}
	m.loadRequest(other, "0/r1")

	// The first request's PreRequest must be untouched by the second load.
	if req.Scripts.PreRequest != "original" {
		t.Errorf("first req PreRequest mutated during second load: %q", req.Scripts.PreRequest)
	}
	if other.Scripts.PreRequest != "" {
		t.Errorf("second req PreRequest = %q, want empty", other.Scripts.PreRequest)
	}
}

// TestScriptsTabClearsOnNilRequest verifies that loadRequest(nil, "")
// blanks both editors and the console panel so the previous request's
// content doesn't bleed visibly into the next selection.
func TestScriptsTabClearsOnNilRequest(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "Bound", Method: model.GET, URL: "https://x/",
		Scripts: model.Scripts{PreRequest: "x;", PostResponse: "y;"},
	}
	m.loadRequest(req, "0/r0")
	m.setScriptConsole([]string{"log line"})

	m.loadRequest(nil, "")
	if m.preScriptEditor.Text != "" {
		t.Errorf("preScriptEditor = %q, want empty", m.preScriptEditor.Text)
	}
	if m.postScriptEditor.Text != "" {
		t.Errorf("postScriptEditor = %q, want empty", m.postScriptEditor.Text)
	}
	if m.scriptConsole.Text != "" {
		t.Errorf("scriptConsole = %q, want empty", m.scriptConsole.Text)
	}
}

// TestSetScriptConsole verifies multi-line joining and the empty-input
// case clears the panel.
func TestSetScriptConsole(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	m.setScriptConsole([]string{"one", "two", "three"})
	if m.scriptConsole.Text != "one\ntwo\nthree" {
		t.Errorf("scriptConsole = %q", m.scriptConsole.Text)
	}
	m.setScriptConsole(nil)
	if m.scriptConsole.Text != "" {
		t.Errorf("after nil: scriptConsole = %q, want empty", m.scriptConsole.Text)
	}
}

// TestSetScriptConsoleTruncates verifies that exceeding
// scriptConsoleMaxLines keeps the most recent entries and prepends a
// truncation notice — so a noisy script can't freeze the widget.
func TestSetScriptConsoleTruncates(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	const n = scriptConsoleMaxLines + 50
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line"
	}
	m.setScriptConsole(lines)
	rendered := m.scriptConsole.Text
	if !strings.HasPrefix(rendered, "[truncated 50 earlier lines]") {
		t.Errorf("expected truncation notice prefix, got: %q", rendered[:80])
	}
	// Total rendered lines = notice + scriptConsoleMaxLines kept.
	got := strings.Count(rendered, "\n") + 1
	want := scriptConsoleMaxLines + 1
	if got != want {
		t.Errorf("line count = %d, want %d", got, want)
	}
}

// TestSessionEnvBridgeAdapter verifies the adapter Get reads through the
// session Resolver (env + overlay) and Set writes only to the overlay.
func TestSessionEnvBridgeAdapter(t *testing.T) {
	sess, _ := session.New("")
	bridge := sessionEnvBridge{s: sess}

	bridge.Set("FOO", "bar")
	if v, ok := bridge.Get("FOO"); !ok || v != "bar" {
		t.Errorf("bridge Get FOO = %q ok=%v, want bar true", v, ok)
	}
	if _, ok := sess.EnvOverlay("FOO"); !ok {
		t.Error("expected overlay to be set by bridge.Set")
	}
}
