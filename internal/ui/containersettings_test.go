package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

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

// newSettingsUI saves a collection (root Bearer auth, one folder "F" holding
// request "R" that inherits, one collection variable) and opens it in a
// MainUI with a real test window. Returns the UI, session, and collection dir.
func newSettingsUI(t *testing.T) (*MainUI, *session.Session, string) {
	t.Helper()
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{
		Name:      "C0",
		Auth:      model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "root-token"}},
		Variables: []model.Variable{{Enabled: true, Key: "base", Value: "v1"}},
		Folders: []model.Folder{{
			Name:      "F",
			Variables: []model.Variable{{Enabled: true, Key: "fk", Value: "f1"}},
			Requests:  []model.Request{{Name: "R", Method: model.GET, URL: "https://x/", Auth: model.Auth{Type: model.AuthInherit}}},
		}},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := session.New(filepath.Join(t.TempDir(), "cfg.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(900, 700))
	t.Cleanup(w.Close)
	m.SetWindow(w)
	return m, s, dir
}

// TestFolderSettingsButtonGating verifies the folder-settings button enables
// only when a folder node is selected.
func TestFolderSettingsButtonGating(t *testing.T) {
	m, _, _ := newSettingsUI(t)

	m.lastSelectedNodeID = "0/f0" // folder
	m.refreshSidebarActions()
	if m.sbFolderSettings.Disabled() {
		t.Error("folder-settings button should be enabled for a folder selection")
	}
	m.lastSelectedNodeID = "0/f0/r0" // request
	m.refreshSidebarActions()
	if !m.sbFolderSettings.Disabled() {
		t.Error("folder-settings button should be disabled for a request selection")
	}
	m.lastSelectedNodeID = "" // nothing
	m.refreshSidebarActions()
	if !m.sbFolderSettings.Disabled() {
		t.Error("folder-settings button should be disabled with no selection")
	}
}

// TestFolderSettingsSavesAuthAndVariables is the feature test: setting auth on
// a folder through its settings dialog persists it, one write covers both
// tabs, and the request inside now inherits it (in memory and after reload).
func TestFolderSettingsSavesAuthAndVariables(t *testing.T) {
	m, s, dir := newSettingsUI(t)
	// Open the inheriting request so its Auth tab's Inherit preview is live.
	m.openOrActivate("0/f0/r0")
	if got := m.authEd.inheritLabel.Text; !strings.Contains(got, "Bearer Token") {
		t.Fatalf("before: inherit preview = %q, want the collection's Bearer", got)
	}

	cs, ok := m.folderSettings("0/f0")
	if !ok {
		t.Fatal("folderSettings(0/f0) not ok")
	}
	if cs.auth.typeSel.Selected != "Inherit from parent" {
		t.Errorf("folder with no own auth should load as Inherit, got %q", cs.auth.typeSel.Selected)
	}
	cs.auth.typeSel.SetSelected("Basic Auth")
	cs.auth.basicUsername.SetText("alice")
	cs.auth.basicPassword.SetText("s3cret")
	cs.save()

	// In-memory: the folder carries the auth and the request inherits it.
	if fa, _ := s.FolderAuth("0/f0"); fa.Type != model.AuthBasic || fa.Basic == nil || fa.Basic.Username != "alice" {
		t.Errorf("FolderAuth after save = %+v", fa)
	}
	if eff := s.EffectiveAuth("0/f0/r0"); eff.Type != model.AuthBasic {
		t.Errorf("EffectiveAuth(request) = %+v, want the folder's Basic", eff)
	}
	if got := m.authEd.inheritLabel.Text; !strings.Contains(got, "Basic Auth") {
		t.Errorf("after: inherit preview = %q, want Basic Auth", got)
	}
	// On disk: auth persisted, existing folder variables untouched, secret externalized + merged back.
	c, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := c.Folders[0]
	if f.Auth.Type != model.AuthBasic || f.Auth.Basic == nil || f.Auth.Basic.Username != "alice" || f.Auth.Basic.Password != "s3cret" {
		t.Errorf("folder auth on disk = %+v", f.Auth)
	}
	if len(f.Variables) != 1 || f.Variables[0].Key != "fk" || f.Variables[0].Value != "f1" {
		t.Errorf("folder variables perturbed by an auth-only save: %+v", f.Variables)
	}
}

// TestFolderSettingsInheritPreview verifies the folder dialog's Inherit panel
// shows what the folder would inherit from the collection root, and that a
// non-folder node yields no dialog.
func TestFolderSettingsInheritPreview(t *testing.T) {
	m, _, _ := newSettingsUI(t)
	cs, ok := m.folderSettings("0/f0")
	if !ok {
		t.Fatal("folderSettings(0/f0) not ok")
	}
	if got := cs.auth.inheritLabel.Text; !strings.Contains(got, "Bearer Token") {
		t.Errorf("inherit preview = %q, want the collection's Bearer", got)
	}
	if _, ok := m.folderSettings("0/f0/r0"); ok {
		t.Error("folderSettings on a request node should be false")
	}
	m.lastSelectedNodeID = "0/f0/r0"
	m.editFolderSettings()
	if m.Status.Text != "Select a folder first" {
		t.Errorf("status = %q, want the select-a-folder hint", m.Status.Text)
	}
}

// TestCollectionSettingsAuth verifies the collection-root dialog omits
// "Inherit from parent" (a root has no parent), loads the root's own auth, and
// persists a change so top-level descendants inherit it.
func TestCollectionSettingsAuth(t *testing.T) {
	m, s, dir := newSettingsUI(t)
	cs, ok := m.collectionSettings(0)
	if !ok {
		t.Fatal("collectionSettings(0) not ok")
	}
	for _, o := range cs.auth.typeSel.Options {
		if o == "Inherit from parent" {
			t.Fatal("collection root auth editor must not offer Inherit")
		}
	}
	if cs.auth.typeSel.Selected != "Bearer Token" || cs.auth.bearerToken.Text != "root-token" {
		t.Errorf("root auth not loaded: type=%q token=%q", cs.auth.typeSel.Selected, cs.auth.bearerToken.Text)
	}
	cs.auth.typeSel.SetSelected("API Key")
	cs.auth.apiKeyName.SetText("X-Key")
	cs.auth.apiKeyValue.SetText("v")
	cs.auth.apiKeyPlacement.SetSelected("Query")
	cs.save()

	if eff := s.EffectiveAuth("0/f0/r0"); eff.Type != model.AuthAPIKey || eff.APIKey.Placement != model.APIKeyQuery {
		t.Errorf("EffectiveAuth(request) = %+v, want the new root API key", eff)
	}
	c, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Type != model.AuthAPIKey || c.Auth.APIKey == nil || c.Auth.APIKey.Name != "X-Key" || c.Auth.APIKey.Value != "v" {
		t.Errorf("root auth on disk = %+v", c.Auth)
	}
	if _, ok := m.collectionSettings(5); ok {
		t.Error("collectionSettings out of range should be false")
	}
}

// TestCollectionSettingsZeroAuthLoadsAsNone verifies a root whose Auth is the
// zero value (a collection created in-app and never saved with auth) shows
// None rather than falling back to the (unavailable) Inherit label.
func TestCollectionSettingsZeroAuthLoadsAsNone(t *testing.T) {
	m, s, _ := newSettingsUI(t)
	s.Collections()[0].Auth = model.Auth{}
	cs, _ := m.collectionSettings(0)
	if cs.auth.typeSel.Selected != "None" {
		t.Errorf("zero root auth loaded as %q, want None", cs.auth.typeSel.Selected)
	}
}

// TestEditCollectionSettingsSavesVariables pins #80's UI editor through the
// real dialog: editing a collection variable on the Variables tab and tapping
// Save persists it back to the collection's YAML (auth untouched).
func TestEditCollectionSettingsSavesVariables(t *testing.T) {
	m, _, dir := newSettingsUI(t)
	w := m.win

	m.editCollectionSettings()
	top := w.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("collection settings dialog did not open")
	}

	var valEntry *widget.Entry
	var saveBtn *widget.Button
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Entry:
			if v.Text == "v1" {
				valEntry = v
			}
		case *widget.Button:
			if v.Text == "Save" {
				saveBtn = v
			}
		}
	})
	if valEntry == nil || saveBtn == nil {
		t.Fatalf("dialog widgets not found (val=%v save=%v)", valEntry, saveBtn)
	}
	valEntry.SetText("v2")
	saveBtn.OnTapped()

	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Variables) != 1 || reloaded.Variables[0].Key != "base" || reloaded.Variables[0].Value != "v2" {
		t.Errorf("collection variable not persisted via editor: %+v", reloaded.Variables)
	}
	if reloaded.Auth.Type != model.AuthBearer || reloaded.Auth.Bearer == nil || reloaded.Auth.Bearer.Token != "root-token" {
		t.Errorf("root auth perturbed by a variables-only save: %+v", reloaded.Auth)
	}
}

// TestEditFolderSettingsOpensDialog verifies the sidebar action opens the
// dialog titled after the folder, with both tabs present.
func TestEditFolderSettingsOpensDialog(t *testing.T) {
	m, _, _ := newSettingsUI(t)
	m.lastSelectedNodeID = "0/f0"
	m.editFolderSettings()
	top := m.win.Canvas().Overlays().Top()
	if top == nil {
		t.Fatal("folder settings dialog did not open")
	}
	var sawTitle bool
	var tabs *container.AppTabs
	walkObjects(top, func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Label:
			if v.Text == "Folder settings: F" {
				sawTitle = true
			}
		case *container.AppTabs:
			tabs = v
		}
	})
	if !sawTitle {
		t.Error("dialog title not found")
	}
	if tabs == nil {
		t.Fatal("settings tabs not found in the dialog")
	}
	if len(tabs.Items) != 2 || tabs.Items[0].Text != "Variables" || tabs.Items[1].Text != "Auth" {
		t.Errorf("tabs = %v, want Variables + Auth", tabs.Items)
	}
}
