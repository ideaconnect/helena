package ui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/history"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

func historyUI(t *testing.T) *MainUI {
	t.Helper()
	test.NewApp()
	sess, err := session.New("") // in-memory history
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	w := test.NewWindow(m.Root())
	t.Cleanup(w.Close)
	m.SetWindow(w)
	return m
}

// TestRecordHistoryOnSend: a completed send is recorded with the resolved
// method/URL and the response summary (#65).
func TestRecordHistoryOnSend(t *testing.T) {
	m := historyUI(t)
	req := model.Request{Method: model.GET, URL: "{{base}}/a"}
	view := chain.View{
		Request:  chain.RequestView{Method: "GET", URL: "https://api/a"}, // resolved
		Response: chain.ResponseView{StatusCode: 200, Size: 12, Duration: 3 * time.Millisecond},
	}
	m.recordHistory(req, view, nil, false)

	got := m.sess.History().Entries()
	if len(got) != 1 {
		t.Fatalf("history len = %d, want 1", len(got))
	}
	e := got[0]
	if e.Method != "GET" || e.URL != "https://api/a" || e.Status != 200 || e.Size != 12 {
		t.Errorf("entry = %+v, want resolved GET https://api/a 200 size 12", e)
	}
	// The stored request keeps the authored (templated) URL for restore/resend.
	if e.Request.URL != "{{base}}/a" {
		t.Errorf("stored request URL = %q, want the authored {{base}}/a", e.Request.URL)
	}
}

// TestRecordHistoryError: a send that never completed HTTP records the error and
// a zero status.
func TestRecordHistoryError(t *testing.T) {
	m := historyUI(t)
	req := model.Request{Method: model.POST, URL: "https://api/down"}
	view := chain.View{Request: chain.RequestView{Method: "POST", URL: "https://api/down"}}
	m.recordHistory(req, view, errString("connection refused"), false)

	e := m.sess.History().Entries()[0]
	if e.Status != 0 || !strings.Contains(e.Err, "connection refused") {
		t.Errorf("errored entry = %+v, want status 0 + the error string", e)
	}
}

// TestRecordHistorySkipsAbort: a user-aborted send (canceled) is not recorded —
// a Stop is not a real send.
func TestRecordHistorySkipsAbort(t *testing.T) {
	m := historyUI(t)
	req := model.Request{Method: model.GET, URL: "https://api/slow"}
	view := chain.View{Request: chain.RequestView{Method: "GET", URL: "https://api/slow"}}
	m.recordHistory(req, view, errString("context canceled"), true)
	if n := m.sess.History().Len(); n != 0 {
		t.Errorf("aborted send recorded: history len = %d, want 0", n)
	}
}

// TestHistoryRestoreOpensTab: restoring an entry opens its request in a new
// scratch tab (the action the History dialog's Restore button performs).
func TestHistoryRestoreOpensTab(t *testing.T) {
	m := historyUI(t)
	before := len(m.tabs)
	entry := history.Entry{Method: "GET", URL: "https://api/a",
		Request: model.Request{Method: model.GET, URL: "https://api/a", Name: "Restored"}}
	m.openScratchWith(entry.Request) // what Restore calls
	if len(m.tabs) != before+1 {
		t.Fatalf("tabs = %d, want %d (restore did not open a tab)", len(m.tabs), before+1)
	}
	if m.currentRequest == nil || m.currentRequest.URL != "https://api/a" {
		t.Errorf("restored current request = %+v, want the history URL", m.currentRequest)
	}
}

// TestShowHistoryOpens: the viewer opens whether or not there is history, without
// panicking.
func TestShowHistoryOpens(t *testing.T) {
	m := historyUI(t)
	m.showHistory() // empty
	if m.win.Canvas().Overlays().Top() == nil {
		t.Fatal("empty history dialog did not open")
	}
	m.win.Canvas().Overlays().Top().Hide()

	m.sess.History().Record(history.Entry{Method: "GET", URL: "https://api/a",
		Request: model.Request{Method: model.GET, URL: "https://api/a"}})
	m.showHistory()
	if m.win.Canvas().Overlays().Top() == nil {
		t.Fatal("populated history dialog did not open")
	}
}

// TestHistorySummary checks the row rendering: a status code, an ERR marker, and
// the method/URL.
func TestHistorySummary(t *testing.T) {
	ok := historySummary(history.Entry{Method: "GET", URL: "https://api/a", Status: 200, Time: time.Now()})
	if !strings.Contains(ok, "GET") || !strings.Contains(ok, "200") || !strings.Contains(ok, "https://api/a") {
		t.Errorf("ok summary = %q", ok)
	}
	bad := historySummary(history.Entry{Method: "POST", URL: "https://api/x", Err: "boom", Time: time.Now()})
	if !strings.Contains(bad, "ERR") {
		t.Errorf("errored summary = %q, want an ERR marker", bad)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
