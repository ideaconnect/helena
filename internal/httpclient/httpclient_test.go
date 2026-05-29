package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestDoGETWithParams verifies that enabled query params merge with URL params and disabled ones are dropped.
func TestDoGETWithParams(t *testing.T) {
	var gotPath, gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query().Encode()
		_, _ = w.Write([]byte("pong"))
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.GET,
		URL:    "{{base}}/ping?z=0",
		Params: []model.KeyValue{
			{Enabled: true, Key: "q", Value: "hi"},
			{Enabled: false, Key: "skip", Value: "x"},
		},
	}
	resp, err := c.Do(context.Background(), req, vars.New(map[string]string{"base": ts.URL}))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "pong" || resp.Size != 4 {
		t.Errorf("status=%d body=%q size=%d", resp.StatusCode, resp.Body, resp.Size)
	}
	if gotPath != "/ping" {
		t.Errorf("path=%q want /ping", gotPath)
	}
	if !strings.Contains(gotQuery, "q=hi") || !strings.Contains(gotQuery, "z=0") || strings.Contains(gotQuery, "skip") {
		t.Errorf("query=%q (want q=hi and z=0, not skip)", gotQuery)
	}
}

// TestPostJSONBodyAndHeaders verifies that {{vars}} resolve in body and headers and that Content-Type is auto-set for JSON.
func TestPostJSONBodyAndHeaders(t *testing.T) {
	var gotBody, gotCT, gotTok string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotCT, gotTok = string(b), r.Header.Get("Content-Type"), r.Header.Get("X-Token")
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method:  model.POST,
		URL:     "{{base}}/users",
		Headers: []model.KeyValue{{Enabled: true, Key: "X-Token", Value: "{{tok}}"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"n":{{n}}}`},
	}
	res := vars.New(map[string]string{"base": ts.URL, "tok": "secret", "n": "42"})
	resp, err := c.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status=%d want 201", resp.StatusCode)
	}
	if gotBody != `{"n":42}` {
		t.Errorf("body=%q want {\"n\":42}", gotBody)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type=%q", gotCT)
	}
	if gotTok != "secret" {
		t.Errorf("x-token=%q want secret", gotTok)
	}
}

// TestUnresolvedVariablesError verifies that Build returns an error naming every unresolved {{variable}}.
func TestUnresolvedVariablesError(t *testing.T) {
	_, err := Build(context.Background(),
		model.Request{Method: model.GET, URL: "{{base}}/{{missing}}"},
		vars.New(map[string]string{"base": "http://x"}), nil)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err=%v, want it to name 'missing'", err)
	}
}

// TestRedirectPolicy verifies that Settings.FollowRedirects toggles between stopping at 302 and following to the final body.
func TestRedirectPolicy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("final")) })
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) })
	ts := httptest.NewServer(mux)
	defer ts.Close()

	noFollow, _ := New(model.Settings{FollowRedirects: false}).Do(
		context.Background(), model.Request{Method: model.GET, URL: ts.URL + "/start"}, nil)
	if noFollow == nil || noFollow.StatusCode != http.StatusFound {
		t.Errorf("no-follow status=%v, want 302", noFollow)
	}

	follow, err := New(model.Settings{FollowRedirects: true}).Do(
		context.Background(), model.Request{Method: model.GET, URL: ts.URL + "/start"}, nil)
	if err != nil {
		t.Fatalf("follow Do: %v", err)
	}
	if follow.StatusCode != 200 || string(follow.Body) != "final" {
		t.Errorf("follow status=%d body=%q", follow.StatusCode, follow.Body)
	}
}

// TestPostFormBodyFallsBackToContent verifies that BodyForm with no structured fields sends raw Content with form Content-Type.
func TestPostFormBodyFallsBackToContent(t *testing.T) {
	var gotBody, gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotCT = string(b), r.Header.Get("Content-Type")
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.POST,
		URL:    ts.URL + "/f",
		Body:   model.Body{Type: model.BodyForm, Content: "a=1&b=hello"},
	}
	if _, err := c.Do(context.Background(), req, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotBody != "a=1&b=hello" {
		t.Errorf("body = %q, want %q", gotBody, "a=1&b=hello")
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", gotCT)
	}
}

// TestBuildAppliesAuthAfterVarResolution verifies that auth credentials
// run through {{var}} substitution and that the resulting Bearer token
// reaches the outgoing Authorization header.
func TestBuildAppliesAuthAfterVarResolution(t *testing.T) {
	resolver := vars.New(map[string]string{"TOKEN": "abc123"})
	req, err := Build(context.Background(), model.Request{
		Method: model.GET,
		URL:    "https://example.com",
		Auth: model.Auth{
			Type:   model.AuthBearer,
			Bearer: &model.BearerAuth{Token: "{{TOKEN}}"},
		},
	}, resolver, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Errorf("Authorization = %q", got)
	}
}

// TestBuildReportsUnresolvedAuthVars verifies that unresolved {{vars}} in
// the auth section show up in the missing-variables error alongside any
// from URL/headers/body.
func TestBuildReportsUnresolvedAuthVars(t *testing.T) {
	_, err := Build(context.Background(), model.Request{
		Method: model.GET,
		URL:    "https://example.com",
		Auth: model.Auth{
			Type:   model.AuthBearer,
			Bearer: &model.BearerAuth{Token: "{{NOT_SET}}"},
		},
	}, vars.New(), nil)
	if err == nil || !strings.Contains(err.Error(), "NOT_SET") {
		t.Errorf("Build err = %v, want one mentioning NOT_SET", err)
	}
}

// TestCORSAdvisory verifies that corsAdvisory warns on missing or mismatched Access-Control-Allow-Origin and stays silent on match or wildcard.
func TestCORSAdvisory(t *testing.T) {
	header := func(allow string) http.Header {
		h := http.Header{}
		if allow != "" {
			h.Set("Access-Control-Allow-Origin", allow)
		}
		return h
	}
	cases := []struct {
		name, origin, allow string
		wantWarn            bool
	}{
		{"no origin", "", "", false},
		{"missing acao", "https://app.example", "", true},
		{"wildcard", "https://app.example", "*", false},
		{"exact match", "https://app.example", "https://app.example", false},
		{"mismatch", "https://app.example", "https://other", true},
	}
	for _, tc := range cases {
		got := corsAdvisory(tc.origin, header(tc.allow))
		if (got != "") != tc.wantWarn {
			t.Errorf("%s: corsAdvisory=%q, wantWarn=%v", tc.name, got, tc.wantWarn)
		}
	}
}
