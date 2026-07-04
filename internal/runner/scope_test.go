package runner

import (
	"context"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestRunScopeWholeVsFolder verifies RunScope("") runs the whole collection
// while a folder id restricts the run to that folder's subtree, with paths kept
// collection-relative (#89).
func TestRunScopeWholeVsFolder(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	get := func(name string) model.Request {
		return model.Request{Name: name, Method: model.GET, URL: srv.URL + "/ok"}
	}
	col := model.Collection{
		Name:     "Scoped",
		Requests: []model.Request{get("Root")},
		Folders: []model.Folder{
			{Name: "Alpha", Requests: []model.Request{get("A1"), get("A2")}},
			{Name: "Beta", Requests: []model.Request{get("B1")}},
		},
	}
	sess := openColl(t, col)

	whole := RunScope(context.Background(), sess, "")
	if len(whole.Results) != 4 {
		t.Fatalf("whole run: %d requests, want 4", len(whole.Results))
	}

	// FolderNodeID resolves the name path; "0/f0" is Alpha.
	alpha, ok := sess.FolderNodeID(0, "Alpha")
	if !ok || alpha != "0/f0" {
		t.Fatalf("FolderNodeID(Alpha) = %q,%v; want 0/f0,true", alpha, ok)
	}
	scoped := RunScope(context.Background(), sess, alpha)
	if len(scoped.Results) != 2 {
		t.Fatalf("Alpha run: %d requests, want 2", len(scoped.Results))
	}
	if byPath(scoped, "Alpha/A1") == nil || byPath(scoped, "Alpha/A2") == nil {
		t.Errorf("Alpha run missing its requests: %+v", scoped.Results)
	}
	if byPath(scoped, "Root") != nil || byPath(scoped, "Beta/B1") != nil {
		t.Errorf("Alpha run leaked out-of-scope requests: %+v", scoped.Results)
	}
}

// TestRunScopeFolderPrefixNotSibling guards the node-id prefix match: scoping to
// "0/f1" must not also pull in a sibling "0/f10" (the trailing-separator check).
func TestRunScopeFolderPrefixNotSibling(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	get := func(name string) model.Request {
		return model.Request{Name: name, Method: model.GET, URL: srv.URL + "/ok"}
	}
	// 11 folders so indices f0..f10 exist; only f1 and f10 carry a request.
	folders := make([]model.Folder, 11)
	for i := range folders {
		folders[i] = model.Folder{Name: "F" + itoa(i)}
	}
	folders[1].Requests = []model.Request{get("One")}
	folders[10].Requests = []model.Request{get("Ten")}
	sess := openColl(t, model.Collection{Name: "Prefix", Folders: folders})

	scoped := RunScope(context.Background(), sess, "0/f1")
	if len(scoped.Results) != 1 {
		t.Fatalf("scope 0/f1: %d requests, want 1 (0/f10 must not match)", len(scoped.Results))
	}
	if byPath(scoped, "F1/One") == nil {
		t.Errorf("scope 0/f1 missing F1/One: %+v", scoped.Results)
	}
}

// itoa avoids strconv import churn in the table above.
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
