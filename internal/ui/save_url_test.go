package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// openSavedRequest stores a single-request collection, opens it, and returns the
// MainUI bound to the live request pointer plus the collection dir.
func openSavedRequest(t *testing.T, req model.Request) (*MainUI, *model.Request, string) {
	t.Helper()
	test.NewApp()
	dir := filepath.Join(t.TempDir(), "c0")
	col := model.Collection{Name: "C0", Requests: []model.Request{req}}
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
	t.Cleanup(func() { w.Close() })
	m.SetWindow(w)
	cols := s.Collections()
	return m, &cols[0].Requests[0], dir
}

// TestOpenThenSaveURLByteIdentical pins #101: opening a request whose stored URL
// carries an inline query and saving it without edits must leave the on-disk URL
// byte-identical (no param reordering / re-encoding / move into Params).
func TestOpenThenSaveURLByteIdentical(t *testing.T) {
	const orig = "https://api.test/path?b=2&a=1&b=3" // unsorted, repeated key, would re-encode
	m, req, dir := openSavedRequest(t, model.Request{Name: "R", Method: model.GET, URL: orig})

	m.loadRequest(req, "0/r0")
	m.saveRequest() // no edits

	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Requests[0]
	if got.URL != orig {
		t.Errorf("open+save mutated the URL:\n got  %q\n want %q (byte-identical)", got.URL, orig)
	}
	if len(got.Params) != 0 {
		t.Errorf("inline query should stay in the URL, not move into Params; got %+v", got.Params)
	}
}

// TestEditedURLPersistsNormalized pins the other side of #101: once the user
// actually edits the URL, the normalized base+Params fold IS persisted.
func TestEditedURLPersistsNormalized(t *testing.T) {
	const orig = "https://api.test/path?b=2&a=1"
	m, req, dir := openSavedRequest(t, model.Request{Name: "R", Method: model.GET, URL: orig})

	m.loadRequest(req, "0/r0")
	m.URL.SetText("https://api.test/path?x=9") // fires OnChanged -> applyURLEdit
	m.saveRequest()

	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Requests[0]
	if got.URL != "https://api.test/path" {
		t.Errorf("edited URL base = %q; want %q", got.URL, "https://api.test/path")
	}
	found := false
	for _, p := range got.Params {
		if p.Key == "x" && p.Value == "9" {
			found = true
		}
	}
	if !found {
		t.Errorf("edited param x=9 not persisted; URL=%q Params=%+v", got.URL, got.Params)
	}
}
