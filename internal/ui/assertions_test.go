package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestAssertionsTabLoadAddDelete verifies the assertion rows reflect the loaded
// request and that Add appends a default row (#88).
func TestAssertionsTabLoadAddDelete(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)
	defer test.NewWindow(m.Root()).Close()

	req := &model.Request{
		Name: "WithAsserts", Method: model.GET, URL: "https://x/",
		Assertions: []model.Assertion{{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"}},
	}
	m.loadRequest(req, "0/r0")
	if got := len(m.assertionRows.Objects); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}

	m.addAssertion()
	if got := len(m.currentRequest.Assertions); got != 2 {
		t.Errorf("Assertions len after Add = %d, want 2", got)
	}
	if last := m.currentRequest.Assertions[1]; !last.Enabled || last.Source != "res.status" || last.Op != "equals" {
		t.Errorf("default added assertion = %+v", last)
	}
}
