package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestOpenScratchWithLoadsCurlRequest pins the #71 UI entry: a request parsed
// from curl opens in a new scratch tab bound to the editor.
func TestOpenScratchWithLoadsCurlRequest(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)

	req, err := importer.FromCurl(`curl -X POST https://api.test/x -H 'Accept: application/json' -d '{"a":1}'`)
	if err != nil {
		t.Fatal(err)
	}
	before := len(m.tabs)
	m.openScratchWith(req)

	if len(m.tabs) != before+1 {
		t.Fatalf("expected one new tab, got %d (was %d)", len(m.tabs), before)
	}
	if m.currentRequest == nil || m.currentRequest.Method != model.POST || m.currentRequest.URL != "https://api.test/x" {
		t.Errorf("scratch request not loaded into editor: %+v", m.currentRequest)
	}
	if m.currentRequest.Body.Type != model.BodyJSON {
		t.Errorf("body type = %q; want JSON", m.currentRequest.Body.Type)
	}
	tab := m.activeTab()
	if tab == nil || !tab.scratch {
		t.Errorf("active tab is not a scratch tab: %+v", tab)
	}
}
