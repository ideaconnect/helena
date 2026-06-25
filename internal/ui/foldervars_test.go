package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// TestWithFolderVarsRequestWins verifies folder vars are folded into the request
// scope with the request's own values winning on a clash (#81).
func TestWithFolderVarsRequestWins(t *testing.T) {
	own := []model.Variable{{Enabled: true, Key: "k", Value: "req"}}
	merged := withFolderVars(map[string]string{"k": "folder", "fonly": "fv"}, own)
	m := enabledRequestVars(merged)
	if m["k"] != "req" {
		t.Errorf("k = %q, want request value to win", m["k"])
	}
	if m["fonly"] != "fv" {
		t.Errorf("fonly = %q, want folder value fv", m["fonly"])
	}
	// No folder vars → the slice is returned unchanged.
	if got := withFolderVars(nil, own); len(got) != 1 || got[0].Key != "k" {
		t.Errorf("empty folder merge changed own: %+v", got)
	}
}

// TestFolderVarsButtonGating verifies the folder-variables button enables only
// when a folder node is selected (#81).
func TestFolderVarsButtonGating(t *testing.T) {
	test.NewApp()
	dir := t.TempDir() + "/c"
	col := model.Collection{Name: "C", Folders: []model.Folder{{
		Name:     "F",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://x/"}},
	}}}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	sess, _ := session.New("")
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	sess.SetActiveCollection(0)
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	m.lastSelectedNodeID = "0/f0" // folder
	m.refreshSidebarActions()
	if m.sbFolderVars.Disabled() {
		t.Error("folder-vars button should be enabled for a folder selection")
	}
	m.lastSelectedNodeID = "0/f0/r0" // request
	m.refreshSidebarActions()
	if !m.sbFolderVars.Disabled() {
		t.Error("folder-vars button should be disabled for a request selection")
	}
	m.lastSelectedNodeID = "" // nothing
	m.refreshSidebarActions()
	if !m.sbFolderVars.Disabled() {
		t.Error("folder-vars button should be disabled with no selection")
	}
}
