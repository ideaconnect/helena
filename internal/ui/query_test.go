package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

func TestSplitURLQuery(t *testing.T) {
	cases := []struct{ in, base, query string }{
		{"https://x/p?a=1&b=2", "https://x/p", "a=1&b=2"},
		{"https://x/p", "https://x/p", ""},
		{"https://x/p?", "https://x/p", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if b, q := splitURLQuery(c.in); b != c.base || q != c.query {
			t.Errorf("splitURLQuery(%q) = (%q,%q), want (%q,%q)", c.in, b, q, c.base, c.query)
		}
	}
}

func TestParseAndBuildQueryRoundTrip(t *testing.T) {
	// Percent-encoding round-trips and {{vars}} survive untouched.
	params := []model.KeyValue{
		{Enabled: true, Key: "q", Value: "hello world"},
		{Enabled: true, Key: "token", Value: "{{auth_token}}"},
		{Enabled: true, Key: "amp", Value: "a&b"},
	}
	q := buildQueryString(params)
	if q != "q=hello+world&token={{auth_token}}&amp=a%26b" {
		t.Fatalf("buildQueryString = %q", q)
	}
	got := parseQueryParams(q)
	if len(got) != len(params) {
		t.Fatalf("parsed %d params, want %d (%+v)", len(got), len(params), got)
	}
	for i := range params {
		if got[i].Key != params[i].Key || got[i].Value != params[i].Value || !got[i].Enabled {
			t.Errorf("round-trip[%d] = %+v, want %+v", i, got[i], params[i])
		}
	}
}

func TestBuildQueryStringSkipsDisabledAndEmpty(t *testing.T) {
	params := []model.KeyValue{
		{Enabled: true, Key: "a", Value: "1"},
		{Enabled: false, Key: "b", Value: "2"}, // disabled → not in URL
		{Enabled: true, Key: "", Value: "3"},   // empty key → skipped
		{Enabled: true, Key: "c", Value: ""},   // empty value → "c="
	}
	if q := buildQueryString(params); q != "a=1&c=" {
		t.Errorf("buildQueryString = %q, want a=1&c=", q)
	}
}

func TestDisplayURL(t *testing.T) {
	if u := displayURL("http://x", []model.KeyValue{{Enabled: true, Key: "a", Value: "1"}}); u != "http://x?a=1" {
		t.Errorf("displayURL = %q", u)
	}
	if u := displayURL("http://x", nil); u != "http://x" {
		t.Errorf("displayURL (no params) = %q, want bare base", u)
	}
	if u := displayURL("http://x", []model.KeyValue{{Enabled: false, Key: "a", Value: "1"}}); u != "http://x" {
		t.Errorf("displayURL (only disabled) = %q, want bare base", u)
	}
}

func TestMergeQueryFromURLPreservesDisabled(t *testing.T) {
	existing := []model.KeyValue{
		{Enabled: true, Key: "a", Value: "old"},
		{Enabled: false, Key: "c", Value: "3"},
	}
	fromURL := []model.KeyValue{
		{Enabled: true, Key: "a", Value: "new"},
		{Enabled: true, Key: "b", Value: "2"},
	}
	out := mergeQueryFromURL(existing, fromURL)
	if len(out) != 3 || out[0].Value != "new" || out[1].Key != "b" {
		t.Fatalf("merge enabled-from-URL wrong: %+v", out)
	}
	if out[2].Key != "c" || out[2].Enabled {
		t.Errorf("disabled row not preserved: %+v", out)
	}
}

// TestQueryURLTwoWaySync drives the live UI both directions: editing the URL
// fills the Query table (base kept separate), and editing the table rewrites
// the URL field — with disabled rows surviving a URL edit.
func TestQueryURLTwoWaySync(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	req := &model.Request{Method: model.GET, URL: "https://api/x"}
	m.loadRequest(req, "n0")
	if m.URL.Text != "https://api/x" {
		t.Fatalf("after load URL field = %q", m.URL.Text)
	}

	// URL → table.
	m.applyURLEdit("https://api/x?a=1&b=two")
	if m.currentRequest.URL != "https://api/x" {
		t.Errorf("base URL = %q, want query stripped", m.currentRequest.URL)
	}
	if len(m.currentRequest.Params) != 2 ||
		m.currentRequest.Params[0].Key != "a" || m.currentRequest.Params[0].Value != "1" ||
		m.currentRequest.Params[1].Key != "b" || m.currentRequest.Params[1].Value != "two" {
		t.Fatalf("params = %+v, want a=1,b=two", m.currentRequest.Params)
	}

	// table → URL field.
	m.currentRequest.Params[0].Value = "ONE"
	m.syncURLFieldFromParams()
	if m.URL.Text != "https://api/x?a=ONE&b=two" {
		t.Errorf("URL field = %q, want a=ONE&b=two", m.URL.Text)
	}

	// A disabled row can't live in the URL but must survive a URL edit.
	m.currentRequest.Params[1].Enabled = false
	m.applyURLEdit("https://api/x?a=ONE&c=3")
	var disabledB bool
	for _, p := range m.currentRequest.Params {
		if p.Key == "b" && !p.Enabled {
			disabledB = true
		}
	}
	if !disabledB {
		t.Errorf("disabled b dropped after URL edit: %+v", m.currentRequest.Params)
	}
}

// TestQueryFoldsStoredURLQueryOnLoad verifies a request whose stored URL still
// carries a query string (or has separate params) shows everything merged in
// the field, with the base kept query-less.
func TestQueryFoldsStoredURLQueryOnLoad(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	req := &model.Request{
		Method: model.GET,
		URL:    "https://api/x?a=1",
		Params: []model.KeyValue{{Enabled: true, Key: "b", Value: "2"}},
	}
	m.loadRequest(req, "n1")
	if m.currentRequest.URL != "https://api/x" {
		t.Errorf("base URL = %q, want query folded out", m.currentRequest.URL)
	}
	if len(m.currentRequest.Params) != 2 {
		t.Fatalf("params = %+v, want a + b", m.currentRequest.Params)
	}
	if m.URL.Text != "https://api/x?a=1&b=2" {
		t.Errorf("URL field = %q, want merged", m.URL.Text)
	}
}
