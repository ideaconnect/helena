package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// openSessionDir writes the sample collection, opens it, and returns the
// session plus the collection directory (which the tab tests need for
// LocateRequest / persistence assertions).
func openSessionDir(t *testing.T) (*Session, string) {
	t.Helper()
	dir := writeSampleCollection(t)
	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	return s, dir
}

// TestLocateRequest verifies LocateRequest resolves a request by its
// persistent ID to the current node ID and a live pointer, at the root and
// nested in a folder, and returns false for an unknown ID / wrong dir / empty
// ID.
func TestLocateRequest(t *testing.T) {
	s, dir := openSessionDir(t)
	health, _ := s.Tree().Request("0/r0")
	create, _ := s.Tree().Request("0/f0/r0")

	nodeID, req, ok := s.LocateRequest(dir, health.ID)
	if !ok || nodeID != "0/r0" || req.Name != "Health" {
		t.Fatalf("LocateRequest(root) = %q %+v ok=%v", nodeID, req, ok)
	}
	// The returned pointer is live: a write through it is visible via Tree.
	req.URL = "https://changed/"
	if got, _ := s.Tree().Request("0/r0"); got.URL != "https://changed/" {
		t.Errorf("LocateRequest pointer is not live: Tree URL = %q", got.URL)
	}

	if nodeID, _, ok := s.LocateRequest(dir, create.ID); !ok || nodeID != "0/f0/r0" {
		t.Errorf("LocateRequest(nested) = %q ok=%v, want 0/f0/r0", nodeID, ok)
	}
	if _, _, ok := s.LocateRequest(dir, "no-such-id"); ok {
		t.Error("LocateRequest(unknown id) ok=true, want false")
	}
	if _, _, ok := s.LocateRequest("/wrong/dir", health.ID); ok {
		t.Error("LocateRequest(wrong dir) ok=true, want false")
	}
	if _, _, ok := s.LocateRequest(dir, ""); ok {
		t.Error("LocateRequest(empty id) ok=true, want false")
	}
}

// TestLocateRequestReDerivesNodeIDAfterDelete verifies LocateRequest returns
// the request's CURRENT node ID after a sibling deletion shifts indices — the
// property tab reconciliation relies on.
func TestLocateRequestReDerivesNodeIDAfterDelete(t *testing.T) {
	s, dir := openSessionDir(t)
	if _, err := s.AddRequest("0", "Probe"); err != nil { // lands at 0/r1
		t.Fatalf("AddRequest: %v", err)
	}
	probe, _ := s.Tree().Request("0/r1")
	probeID := probe.ID

	if err := s.DeleteItem("0/r0"); err != nil { // Health gone; Probe shifts to 0/r0
		t.Fatalf("DeleteItem: %v", err)
	}
	nodeID, req, ok := s.LocateRequest(dir, probeID)
	if !ok || nodeID != "0/r0" || req.Name != "Probe" {
		t.Errorf("LocateRequest after delete = %q %+v ok=%v, want 0/r0 Probe", nodeID, req, ok)
	}
}

// TestLocateRequestScopedToOwningCollection verifies that two collections
// sharing a Request.ID (e.g. a collection forked on disk, both opened) resolve
// to their OWN request, never each other's.
func TestLocateRequestScopedToOwningCollection(t *testing.T) {
	const shared = "shared-request-id"
	dirA := writeCollectionWithRequestID(t, "colA", "ReqA", shared)
	dirB := writeCollectionWithRequestID(t, "colB", "ReqB", shared)

	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dirA); err != nil {
		t.Fatalf("OpenCollection A: %v", err)
	}
	if err := s.OpenCollection(dirB); err != nil {
		t.Fatalf("OpenCollection B: %v", err)
	}

	if _, req, ok := s.LocateRequest(dirA, shared); !ok || req.Name != "ReqA" {
		t.Errorf("LocateRequest(A) = %+v ok=%v, want ReqA", req, ok)
	}
	if _, req, ok := s.LocateRequest(dirB, shared); !ok || req.Name != "ReqB" {
		t.Errorf("LocateRequest(B) = %+v ok=%v, want ReqB", req, ok)
	}
}

// TestAddRequestValue verifies the populated insert persists every field,
// mints a fresh ID, returns the right node ID, and writes to disk.
func TestAddRequestValue(t *testing.T) {
	s, dir := openSessionDir(t)
	in := model.Request{
		ID:      "ignored-scratch-id",
		Name:    "Created",
		Method:  model.PUT,
		URL:     "https://api/created",
		Headers: []model.KeyValue{{Enabled: true, Key: "X-A", Value: "1"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
	nodeID, err := s.AddRequestValue("0", in)
	if err != nil {
		t.Fatalf("AddRequestValue: %v", err)
	}
	if nodeID != "0/r1" {
		t.Errorf("node id = %q, want 0/r1", nodeID)
	}
	got, ok := s.Tree().Request(nodeID)
	if !ok {
		t.Fatal("new request not found in tree")
	}
	if got.Name != "Created" || got.Method != model.PUT || got.URL != "https://api/created" ||
		len(got.Headers) != 1 || got.Body.Content != `{"a":1}` {
		t.Errorf("stored request = %+v", got)
	}
	if got.ID == "" || got.ID == "ignored-scratch-id" {
		t.Errorf("ID = %q, want a fresh minted ID", got.ID)
	}
	// Persisted to disk: a fresh load sees it.
	reloaded, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Requests) != 2 || reloaded.Requests[1].Name != "Created" {
		t.Errorf("on-disk requests = %+v", reloaded.Requests)
	}
}

// TestAddRequestValueErrors verifies invalid-parent and empty-name rejection.
func TestAddRequestValueErrors(t *testing.T) {
	s, _ := openSessionDir(t)
	if _, err := s.AddRequestValue("99", model.Request{Name: "X"}); err == nil {
		t.Error("AddRequestValue(invalid parent) err=nil, want error")
	}
	if _, err := s.AddRequestValue("0", model.Request{Name: "  "}); err == nil {
		t.Error("AddRequestValue(empty name) err=nil, want error")
	}
}

// TestAddRequestValueDefaultsMethod verifies a request with no method is stored
// as GET.
func TestAddRequestValueDefaultsMethod(t *testing.T) {
	s, _ := openSessionDir(t)
	id, err := s.AddRequestValue("0", model.Request{Name: "NoMethod"})
	if err != nil {
		t.Fatalf("AddRequestValue: %v", err)
	}
	if r, _ := s.Tree().Request(id); r.Method != model.GET {
		t.Errorf("method = %q, want GET", r.Method)
	}
}

// TestSetOpenTabsClampsActiveIndex verifies an out-of-range active index is
// clamped to 0 by both the setter and the getter.
func TestSetOpenTabsClampsActiveIndex(t *testing.T) {
	s, dir := openSessionDir(t)
	health, _ := s.Tree().Request("0/r0")
	s.SetOpenTabs([]config.UIOpenTab{{Collection: dir, RequestID: health.ID}}, 99)
	if _, active := s.OpenTabs(); active != 0 {
		t.Errorf("active after out-of-range set = %d, want 0", active)
	}
	// Getter also defends against an out-of-range persisted index.
	s.cfg.UI.ActiveTab = 99
	if _, active := s.OpenTabs(); active != 0 {
		t.Errorf("active after out-of-range persisted = %d, want 0", active)
	}
}

// TestContainerPaths verifies the destination list covers the collection root
// and every folder, in tree order, with collection-qualified labels.
func TestContainerPaths(t *testing.T) {
	s, _ := openSessionDir(t)
	got := s.ContainerPaths()
	want := []ContainerRef{
		{Label: "Demo API", NodeID: "0"},
		{Label: "Demo API / Users", NodeID: "0/f0"},
	}
	if len(got) != len(want) {
		t.Fatalf("ContainerPaths len = %d (%+v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ContainerPaths[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestCollectionDir verifies the index→dir helper and its out-of-range guards.
func TestCollectionDir(t *testing.T) {
	s, dir := openSessionDir(t)
	if got := s.CollectionDir(0); got != dir {
		t.Errorf("CollectionDir(0) = %q, want %q", got, dir)
	}
	if got := s.CollectionDir(1); got != "" {
		t.Errorf("CollectionDir(1) = %q, want empty", got)
	}
	if got := s.CollectionDir(-1); got != "" {
		t.Errorf("CollectionDir(-1) = %q, want empty", got)
	}
}

// TestOpenTabsRoundTrip verifies SetOpenTabs persists the tab set + active
// index across a fresh session and clears the legacy OpenRequest.
func TestOpenTabsRoundTrip(t *testing.T) {
	s, dir := openSessionDir(t)
	cfgPath := s.cfgPath
	health, _ := s.Tree().Request("0/r0")
	create, _ := s.Tree().Request("0/f0/r0")
	s.SetOpenRequest("0/r0") // legacy state that SetOpenTabs must supersede
	s.SetOpenTabs([]config.UIOpenTab{
		{Collection: dir, RequestID: health.ID},
		{Collection: dir, RequestID: create.ID},
	}, 1)

	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	tabs, active := s2.OpenTabs()
	if len(tabs) != 2 || active != 1 {
		t.Fatalf("OpenTabs = %+v active=%d, want 2 tabs active 1", tabs, active)
	}
	if tabs[0].RequestID != health.ID || tabs[1].RequestID != create.ID {
		t.Errorf("restored tab IDs = %+v", tabs)
	}
	if s2.OpenRequest() != "" {
		t.Errorf("OpenRequest = %q, want cleared by SetOpenTabs", s2.OpenRequest())
	}

	// Empty slice clears the tab state.
	s2.SetOpenTabs(nil, 0)
	if tabs, _ := s2.OpenTabs(); tabs != nil {
		t.Errorf("OpenTabs after clear = %+v, want nil", tabs)
	}
}

// TestOpenTabsStableAcrossRequestReordering verifies a persisted tab restores
// to the correct request by ID even after a sibling deletion renumbers node
// paths — the reason tabs are anchored by Request.ID, not node path.
func TestOpenTabsStableAcrossRequestReordering(t *testing.T) {
	s, dir := openSessionDir(t)
	cfgPath := s.cfgPath
	if _, err := s.AddRequest("0", "Probe"); err != nil { // 0/r1
		t.Fatalf("AddRequest: %v", err)
	}
	probe, _ := s.Tree().Request("0/r1")
	probeID := probe.ID
	s.SetOpenTabs([]config.UIOpenTab{{Collection: dir, RequestID: probeID}}, 0)

	if err := s.DeleteItem("0/r0"); err != nil { // Probe shifts to 0/r0
		t.Fatalf("DeleteItem: %v", err)
	}

	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	tabs, _ := s2.OpenTabs()
	if len(tabs) != 1 || tabs[0].RequestID != probeID {
		t.Fatalf("restored tabs = %+v, want one tab for Probe", tabs)
	}
	nodeID, req, ok := s2.LocateRequest(tabs[0].Collection, tabs[0].RequestID)
	if !ok || nodeID != "0/r0" || req.Name != "Probe" {
		t.Errorf("restored tab resolves to %q %+v ok=%v, want 0/r0 Probe", nodeID, req, ok)
	}
}

// writeCollectionWithRequestID writes a one-request collection carrying an
// explicit Request.ID, used to exercise the cross-collection ID-collision
// scoping of LocateRequest.
func writeCollectionWithRequestID(t *testing.T, colName, reqName, id string) string {
	t.Helper()
	c := model.Collection{
		Name:     colName,
		Requests: []model.Request{{ID: id, Name: reqName, Method: model.GET, URL: "https://x/" + reqName}},
	}
	dir := filepath.Join(t.TempDir(), colName)
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save %s: %v", colName, err)
	}
	return dir
}
