package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/idct/helena/internal/session"
)

// TestSidebarToolbarWiring verifies the node-action toolbar buttons exist, are
// wired to a handler, and that refreshSidebarActions enables/disables them by
// selection: add request/folder always enabled; rename/clone/delete need a
// selection (clone only a request).
func TestSidebarToolbarWiring(t *testing.T) {
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)

	buttons := map[string]*ttwidget.Button{
		"addReq":    m.sbAddReq,
		"addFolder": m.sbAddFolder,
		"rename":    m.sbRename,
		"clone":     m.sbClone,
		"delete":    m.sbDelete,
	}
	for name, b := range buttons {
		if b == nil {
			t.Fatalf("%s button is nil", name)
		}
		if b.OnTapped == nil {
			t.Errorf("%s button has no handler", name)
		}
	}

	// Initial: nothing selected → add request/folder enabled, rename/clone/
	// delete disabled.
	if m.sbAddReq.Disabled() || m.sbAddFolder.Disabled() {
		t.Error("add request/folder should be enabled with no selection")
	}
	if !m.sbRename.Disabled() || !m.sbDelete.Disabled() || !m.sbClone.Disabled() {
		t.Error("rename/clone/delete should be disabled with no selection")
	}

	// A selected collection ("0", no "/") → rename + delete enable, but clone
	// stays disabled (whole collections aren't duplicable).
	m.lastSelectedNodeID = "0"
	m.refreshSidebarActions()
	if m.sbRename.Disabled() || m.sbDelete.Disabled() {
		t.Error("rename/delete should enable when a node is selected")
	}
	if !m.sbClone.Disabled() {
		t.Error("clone should stay disabled for a collection selection")
	}

	// A selected node (folder or request — id contains "/") → clone enables.
	m.lastSelectedNodeID = "0/r0"
	m.refreshSidebarActions()
	if m.sbClone.Disabled() {
		t.Error("clone should enable for a folder/request selection")
	}

	// Selection cleared → all three disable again.
	m.lastSelectedNodeID = ""
	m.refreshSidebarActions()
	if !m.sbRename.Disabled() || !m.sbDelete.Disabled() {
		t.Error("rename/delete should disable when selection clears")
	}
}
