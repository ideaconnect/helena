package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// pathParamRowCount returns how many path-parameter rows the Path tab is
// currently showing.
func pathParamRowCount(m *MainUI) int {
	return len(m.pathParamsRows.Objects)
}

// pathParamValueEntry returns the value entry widget of the idx-th Path row.
func pathParamValueEntry(t *testing.T, m *MainUI, idx int) *shortcutEntry {
	t.Helper()
	border, ok := m.pathParamsRows.Objects[idx].(*fyne.Container)
	if !ok {
		t.Fatalf("row %d is not a container", idx)
	}
	for _, o := range border.Objects {
		if e, ok := o.(*shortcutEntry); ok {
			return e
		}
	}
	t.Fatalf("row %d has no value entry", idx)
	return nil
}

// TestPathTabDerivesRowsFromURL verifies loadRequest lists one Path row per
// {name} token in the URL, in order, and prefills stored values.
func TestPathTabDerivesRowsFromURL(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{
		Method:     model.GET,
		URL:        "{{base_url}}/users/{userId}/orders/{orderId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "userId", Value: "7"}},
	}
	m.loadRequest(req, "id-1")

	if got := pathParamRowCount(m); got != 2 {
		t.Fatalf("path rows = %d, want 2 (userId, orderId)", got)
	}
	if e := pathParamValueEntry(t, m, 0); e.Text != "7" {
		t.Errorf("userId value = %q, want 7", e.Text)
	}
	if e := pathParamValueEntry(t, m, 1); e.Text != "" {
		t.Errorf("orderId value = %q, want empty (not yet filled)", e.Text)
	}
	if m.pathParamsHelp.Visible() {
		t.Errorf("help label should be hidden when path params exist")
	}
}

// TestPathTabHelpWhenNoTokens verifies the tab shows its help text and no rows
// when the URL carries no {name} placeholders.
func TestPathTabHelpWhenNoTokens(t *testing.T) {
	m := newResponseUI(t)
	m.loadRequest(&model.Request{Method: model.GET, URL: "https://api.example.com/ping"}, "id-1")
	if got := pathParamRowCount(m); got != 0 {
		t.Errorf("path rows = %d, want 0", got)
	}
	if !m.pathParamsHelp.Visible() {
		t.Errorf("help label should be visible when no path params exist")
	}
}

// TestPathTabViewingDoesNotMutate verifies merely loading a request that has a
// {name} token but no stored PathParams entry leaves PathParams empty — the tab
// must not synthesize entries just by being shown (keeps Save byte-identical).
func TestPathTabViewingDoesNotMutate(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Method: model.GET, URL: "https://api.example.com/bag/{bagId}"}
	m.loadRequest(req, "id-1")
	if len(req.PathParams) != 0 {
		t.Errorf("PathParams = %+v, want empty after a view-only load", req.PathParams)
	}
}

// TestPathParamValueWriteBack verifies typing a value upserts it into the loaded
// request's PathParams.
func TestPathParamValueWriteBack(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Method: model.GET, URL: "https://api.example.com/bag/{bagId}"}
	m.loadRequest(req, "id-1")

	pathParamValueEntry(t, m, 0).SetText("42")
	if len(req.PathParams) != 1 || req.PathParams[0].Key != "bagId" || req.PathParams[0].Value != "42" {
		t.Fatalf("PathParams = %+v, want bagId=42", req.PathParams)
	}
	if !req.PathParams[0].Enabled {
		t.Errorf("new path param should default to enabled")
	}
}

// TestPathTabReconcilesOnURLEdit verifies editing the URL to add a token adds a
// row live, preserving an already-filled sibling's value.
func TestPathTabReconcilesOnURLEdit(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{Method: model.GET, URL: "https://api.example.com/bag/{bagId}"}
	m.loadRequest(req, "id-1")
	pathParamValueEntry(t, m, 0).SetText("42")

	// Simulate a user URL edit adding a second token.
	m.URL.SetText("https://api.example.com/bag/{bagId}/item/{itemId}")

	if got := pathParamRowCount(m); got != 2 {
		t.Fatalf("path rows after URL edit = %d, want 2", got)
	}
	if e := pathParamValueEntry(t, m, 0); e.Text != "42" {
		t.Errorf("bagId value lost on URL edit: %q", e.Text)
	}
}

// TestPathParamEnableToggleWriteBack verifies unchecking a row disables the
// stored path param (so Build leaves its token literal).
func TestPathParamEnableToggleWriteBack(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/bag/{bagId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "42"}},
	}
	m.loadRequest(req, "id-1")

	row := m.pathParamsRows.Objects[0].(*fyne.Container)
	var check *widget.Check
	for _, obj := range flattenObjects(row) {
		if c, ok := obj.(*widget.Check); ok {
			check = c
		}
	}
	if check == nil {
		t.Fatal("no enable check on path row")
	}
	check.SetChecked(false)
	if req.PathParams[0].Enabled {
		t.Errorf("unchecking the row should disable the path param")
	}
}

// TestURLPreviewShowsFilledAndUnfilledPathParams verifies the preview fills a
// filled token and flags an unfilled one.
func TestURLPreviewShowsFilledAndUnfilledPathParams(t *testing.T) {
	m := newResponseUI(t)
	req := &model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/u/{userId}/o/{orderId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "userId", Value: "7"}},
	}
	m.loadRequest(req, "id-1")
	m.updateURLPreview()

	got := m.urlPreview.Text
	if !strings.Contains(got, "orderId") {
		t.Errorf("preview = %q, want it to flag the unfilled orderId", got)
	}

	// Fill the last one too; the preview should now show the resolved arrow.
	m.upsertPathParam("orderId", func(p *model.KeyValue) { p.Value = "9" })
	m.updateURLPreview()
	got = m.urlPreview.Text
	if !strings.Contains(got, "/u/7/o/9") {
		t.Errorf("preview = %q, want the filled URL", got)
	}
}

// TestURLPreviewIgnoresBraceInQuery verifies a single-brace {name} in the query
// segment is NOT treated as a path parameter by the preview — matching Build,
// which only fills tokens in the base path. Pins the review's parity finding.
func TestURLPreviewIgnoresBraceInQuery(t *testing.T) {
	m := newResponseUI(t)
	// The query folds into Params; the base has no {name} token.
	m.loadRequest(&model.Request{Method: model.GET, URL: "https://api.example.com/search?tag={x}"}, "id-1")
	m.updateURLPreview()
	if got := m.urlPreview.Text; strings.Contains(got, "Path params to fill") {
		t.Errorf("preview = %q, want no path-param flag for a {name} in the query", got)
	}
	if got := pathParamRowCount(m); got != 0 {
		t.Errorf("path rows = %d, want 0 for a query-only {name}", got)
	}
}

// flattenObjects returns every canvas object under c, recursively.
func flattenObjects(c *fyne.Container) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, o := range c.Objects {
		out = append(out, o)
		if sub, ok := o.(*fyne.Container); ok {
			out = append(out, flattenObjects(sub)...)
		}
	}
	return out
}
