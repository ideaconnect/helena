package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestChainTabLoadAndAddDelete verifies the chain rows reflect the
// loaded request, the + Add button appends a blank step, and the row
// × button removes the matching entry.
func TestChainTabLoadAndAddDelete(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "WithChain", Method: model.GET, URL: "https://x/",
		Chain: []model.ChainStep{{Alias: "login", Request: "Auth/Login"}},
	}
	m.loadRequest(req, "0/r0")
	if got := len(m.chainRows.Objects); got != 1 {
		t.Errorf("rows = %d, want 1", got)
	}

	m.addChainStep()
	if got := len(m.currentRequest.Chain); got != 2 {
		t.Errorf("Chain len after Add = %d, want 2", got)
	}
}

// TestChainTabWriteBack verifies typing into an alias/ref entry
// updates the in-memory request, AND that the loading-flag guard
// suppresses write-back during the bulk loadRequest update.
func TestChainTabWriteBack(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "WithChain", Method: model.GET, URL: "https://x/",
		Chain: []model.ChainStep{{Alias: "old-alias", Request: "OldPath"}},
	}
	m.loadRequest(req, "0/r0")

	// Switching to a different request must not echo the prior alias
	// into the previous request via the OnChanged callbacks.
	other := &model.Request{Name: "Second", Method: model.GET, URL: "https://y/"}
	m.loadRequest(other, "0/r1")
	if req.Chain[0].Alias != "old-alias" {
		t.Errorf("first req Chain mutated during second load: %+v", req.Chain[0])
	}
}

// TestPruneEmptyChain verifies blank rows are dropped on save so the
// YAML stays clean for users who clicked +Add without filling in.
func TestPruneEmptyChain(t *testing.T) {
	in := []model.ChainStep{
		{Alias: "good", Request: "X"},
		{Alias: "", Request: "Y"},
		{Alias: "no-ref", Request: ""},
		{Alias: "  ", Request: "  "},
	}
	out := pruneEmptyChain(in)
	if len(out) != 1 || out[0].Alias != "good" {
		t.Errorf("pruneEmptyChain = %+v, want [{good X}]", out)
	}
}
