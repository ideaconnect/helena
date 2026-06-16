package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestPanicResponseIsError verifies the synthetic response built on a recovered
// Send panic is an error carrying the panic value (#110).
func TestPanicResponseIsError(t *testing.T) {
	r := panicResponse("boom")
	if !r.isError {
		t.Error("panicResponse is not flagged isError")
	}
	if !strings.Contains(r.errText, "boom") || !strings.Contains(r.status, "boom") {
		t.Errorf("panicResponse dropped the panic value: %+v", r)
	}
}

// TestRecoveredPanicReplacesStaleTabResponse verifies that routing a panic
// through deliverResponse replaces the originating tab's previously cached
// response rather than leaving the stale one in place (#110).
func TestRecoveredPanicReplacesStaleTabResponse(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	stale := &tabResponse{rawBody: `{"ok":true}`, status: "200 OK"}
	tab := &openTab{requestID: "r1", resp: stale}
	m.tabs = []*openTab{tab}
	m.activeTabIdx = 0
	if m.activeTab() != tab {
		t.Fatal("setup: tab is not the active tab")
	}

	m.deliverResponse(tab, panicResponse("kaboom"))

	if tab.resp == stale {
		t.Fatal("stale cached response was not replaced after a recovered panic")
	}
	if !tab.resp.isError || !strings.Contains(tab.resp.errText, "kaboom") {
		t.Errorf("originating tab does not reflect the panic: %+v", tab.resp)
	}
}
