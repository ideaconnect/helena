package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
)

// activeTabName returns the name text rendered on the active tab's widget.
func activeTabName(t *testing.T, m *MainUI) string {
	t.Helper()
	rt := m.tabWidgets[m.activeTab()]
	if rt == nil {
		t.Fatal("active tab has no widget")
	}
	return rt.name.Text
}

// TestTabDirtyAsteriskLifecycle drives the real editor: a pristine tab has no
// asterisk, an edit adds one, and Save clears it.
func TestTabDirtyAsteriskLifecycle(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")

	if name := activeTabName(t, m); strings.Contains(name, "*") {
		t.Fatalf("pristine tab shows an asterisk: %q", name)
	}

	// Edit through the real URL field so the OnChanged hook fires.
	w.Canvas().Focus(m.URL)
	test.Type(m.URL, "/edited")
	if !m.isTabDirty(m.activeTab()) {
		t.Fatal("tab not dirty after an edit")
	}
	if name := activeTabName(t, m); !strings.HasSuffix(name, " *") {
		t.Errorf("edited tab missing asterisk: %q", name)
	}

	// Saving rebaselines the tab, clearing the marker.
	m.saveRequest()
	if m.isTabDirty(m.activeTab()) {
		t.Fatal("tab still dirty after Save")
	}
	if name := activeTabName(t, m); strings.Contains(name, "*") {
		t.Errorf("saved tab still shows an asterisk: %q", name)
	}
}

// TestTabDirtyClearsOnUndoToBaseline verifies the marker is a live value
// comparison, not a one-way flag: editing then reverting to the saved text
// clears the asterisk.
func TestTabDirtyClearsOnUndoToBaseline(t *testing.T) {
	m, _, _ := newTabUI(t)
	w := test.NewWindow(m.Root())
	defer w.Close()
	m.SetWindow(w)
	m.openOrActivate("0/r0")
	orig := m.currentRequest.URL

	w.Canvas().Focus(m.URL)
	test.Type(m.URL, "X")
	if !m.isTabDirty(m.activeTab()) {
		t.Fatal("tab not dirty after edit")
	}
	// Revert the field to the original text; the marker must clear.
	m.URL.SetText(displayURL(orig, m.currentRequest.Params))
	if m.isTabDirty(m.activeTab()) {
		t.Errorf("tab still dirty after reverting to the saved value")
	}
	if name := activeTabName(t, m); strings.Contains(name, "*") {
		t.Errorf("reverted tab still shows an asterisk: %q", name)
	}
}

// TestIsTabDirtyScratch verifies a scratch tab is dirty exactly when it holds
// content worth saving.
func TestIsTabDirtyScratch(t *testing.T) {
	m := newResponseUI(t)
	empty := &openTab{scratch: true, scratchReq: &model.Request{Method: model.GET}}
	if m.isTabDirty(empty) {
		t.Error("an empty scratch tab should be clean")
	}
	filled := &openTab{scratch: true, scratchReq: &model.Request{Method: model.GET, URL: "https://x"}}
	if !m.isTabDirty(filled) {
		t.Error("a scratch tab with a URL should be dirty")
	}
}
