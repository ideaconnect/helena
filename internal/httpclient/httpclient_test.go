package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestReadCapped verifies the response-body cap: bodies within the limit read
// fully and unflagged; bodies past it are cut to exactly the cap and flagged.
func TestReadCapped(t *testing.T) {
	if d, tr, err := readCapped(strings.NewReader("abc"), 8); err != nil || tr || string(d) != "abc" {
		t.Errorf("within cap: data=%q trunc=%v err=%v", d, tr, err)
	}
	if d, tr, err := readCapped(strings.NewReader("01234567"), 8); err != nil || tr || string(d) != "01234567" {
		t.Errorf("at cap: data=%q trunc=%v err=%v", d, tr, err)
	}
	if d, tr, err := readCapped(strings.NewReader("0123456789"), 8); err != nil || !tr || string(d) != "01234567" {
		t.Errorf("over cap: data=%q trunc=%v err=%v", d, tr, err)
	}
}

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
	_, _, err := Build(context.Background(),
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
	req, _, err := Build(context.Background(), model.Request{
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
	_, _, err := Build(context.Background(), model.Request{
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

// TestBuildNilResolverSubstitutesNothing verifies that Build tolerates
// a nil resolver — the request is sent verbatim and any unresolved
// {{vars}} surface as the standard "unresolved variables" error.
func TestBuildNilResolverSubstitutesNothing(t *testing.T) {
	req, body, err := Build(context.Background(),
		model.Request{Method: model.GET, URL: "http://x/"}, nil, nil)
	if err != nil {
		t.Fatalf("Build with nil resolver: %v", err)
	}
	if req == nil || req.URL.String() != "http://x/" {
		t.Errorf("req.URL = %v, want http://x/", req.URL)
	}
	if body != nil {
		t.Errorf("body = %q, want nil for GET", body)
	}
}

// TestBuildMultipartReturnsError verifies that multipart body type is
// rejected — the feature is documented as not-yet-supported and the
// error surfaces cleanly from Build.
func TestBuildMultipartReturnsError(t *testing.T) {
	_, _, err := Build(context.Background(),
		model.Request{Method: model.POST, URL: "http://x/",
			Body: model.Body{Type: model.BodyMultipart}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "multipart") {
		t.Errorf("expected multipart-not-supported error, got %v", err)
	}
}

// TestBuildInvalidURLReturnsError verifies that a URL that survives
// var resolution but can't be parsed by url.Parse surfaces a clear
// error rather than constructing a broken *http.Request.
func TestBuildInvalidURLReturnsError(t *testing.T) {
	_, _, err := Build(context.Background(),
		model.Request{Method: model.GET, URL: "http://[::invalid"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("expected invalid URL error, got %v", err)
	}
}

// TestBuildHostHeaderOverridesReqHost verifies that a "Host" header in
// the request sets req.Host (which Go's transport reads separately
// from the Host header on the wire) rather than getting Added to the
// normal header set.
func TestBuildHostHeaderOverridesReqHost(t *testing.T) {
	req, _, err := Build(context.Background(),
		model.Request{Method: model.GET, URL: "http://upstream/",
			Headers: []model.KeyValue{{Enabled: true, Key: "Host", Value: "virtual.example"}}}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.Host != "virtual.example" {
		t.Errorf("req.Host = %q, want virtual.example", req.Host)
	}
	if got := req.Header.Get("Host"); got != "" {
		t.Errorf("req.Header[Host] = %q, want empty (set via Host field)", got)
	}
}

// TestBuildEmptyHeaderKeySkipped verifies that an enabled header with
// an empty key is dropped on the way out rather than panicking inside
// net/http.
func TestBuildEmptyHeaderKeySkipped(t *testing.T) {
	req, _, err := Build(context.Background(),
		model.Request{Method: model.GET, URL: "http://x/",
			Headers: []model.KeyValue{
				{Enabled: true, Key: "", Value: "ignored"},
				{Enabled: true, Key: "X-Real", Value: "kept"},
			}}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.Header.Get("X-Real") != "kept" {
		t.Errorf("X-Real header dropped: %+v", req.Header)
	}
}

// TestBuildUnknownBodyTypeFallsBackToContent verifies that a custom /
// unrecognised BodyType still produces a body from Content with no
// Content-Type set — the open-collection spec allows extensions and
// we shouldn't refuse to send.
func TestBuildUnknownBodyTypeFallsBackToContent(t *testing.T) {
	req, body, err := Build(context.Background(),
		model.Request{Method: model.POST, URL: "http://x/",
			Body: model.Body{Type: model.BodyType("custom"), Content: "payload"}}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if string(body) != "payload" {
		t.Errorf("body = %q, want payload", body)
	}
	if got := req.Header.Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want empty for unknown body type", got)
	}
}

// TestSetOAuth2ResolverStores verifies the resolver setter actually
// installs the supplied resolver on the Client (it's then consumed
// during Do via the captured value).
func TestSetOAuth2ResolverStores(t *testing.T) {
	c := New(model.DefaultSettings())
	if c.oauth2Resolver != nil {
		t.Fatal("oauth2Resolver should be nil before SetOAuth2Resolver")
	}
	fake := fakeOAuth2Resolver{}
	c.SetOAuth2Resolver(fake)
	if c.oauth2Resolver == nil {
		t.Error("SetOAuth2Resolver did not install the resolver")
	}
}

// fakeOAuth2Resolver satisfies auth.OAuth2Resolver for the
// SetOAuth2Resolver coverage test. The interface method is never
// called in that test — only the install path matters.
type fakeOAuth2Resolver struct{}

func (fakeOAuth2Resolver) Token(context.Context, model.OAuth2Auth) (string, error) {
	return "", nil
}

// TestDoNetworkErrorSurfaces verifies that a Do against an
// unreachable URL surfaces the underlying transport error rather
// than swallowing it into a zero Response.
func TestDoNetworkErrorSurfaces(t *testing.T) {
	c := New(model.Settings{TimeoutSeconds: 1})
	// 127.0.0.1:1 is the discard port — connect fails fast.
	_, err := c.Do(context.Background(),
		model.Request{Method: model.GET, URL: "http://127.0.0.1:1/"}, nil)
	if err == nil {
		t.Error("expected network error, got nil")
	}
}

// TestDoEmitsCORSWarningWhenEnabled verifies that Settings.CORSWarning
// turns on the corsAdvisory check, populating Response.CORSWarning
// when the response's CORS headers would block the Origin.
func TestDoEmitsCORSWarningWhenEnabled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately mismatched ACAO so the advisory fires.
		w.Header().Set("Access-Control-Allow-Origin", "https://other.example")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()
	c := New(model.Settings{CORSWarning: true})
	resp, err := c.Do(context.Background(),
		model.Request{Method: model.GET, URL: ts.URL,
			Headers: []model.KeyValue{{Enabled: true, Key: "Origin", Value: "https://app.example"}}}, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.CORSWarning == "" {
		t.Error("expected non-empty CORSWarning when Settings.CORSWarning is true")
	}
}

// TestNewAppliesSettingsTimeout verifies the underlying http.Client's
// Timeout field reflects Settings.TimeoutSeconds: a positive value
// becomes the exact duration, zero stays zero (no timeout). Kills
// the boundary + negation mutations on the `if s.TimeoutSeconds > 0`
// check and the arithmetic mutation on the seconds → Duration cast.
func TestNewAppliesSettingsTimeout(t *testing.T) {
	if got := New(model.Settings{TimeoutSeconds: 5}).http.Timeout; got != 5*time.Second {
		t.Errorf("TimeoutSeconds=5 → http.Timeout = %v, want 5s", got)
	}
	if got := New(model.Settings{TimeoutSeconds: 0}).http.Timeout; got != 0 {
		t.Errorf("TimeoutSeconds=0 → http.Timeout = %v, want 0 (no timeout)", got)
	}
	if got := New(model.Settings{TimeoutSeconds: 12}).http.Timeout; got != 12*time.Second {
		t.Errorf("TimeoutSeconds=12 → http.Timeout = %v, want 12s", got)
	}
}

// TestBuildDefaultsMissingMethodToGET verifies the empty-Method
// branch in Build: when model.Request.Method is "", the produced
// *http.Request uses "GET". Kills the NEGATION mutation flipping
// `method == ""` to `method != ""` (which would otherwise leave
// req.Method empty and trigger an http transport error later).
func TestBuildDefaultsMissingMethodToGET(t *testing.T) {
	req, _, err := Build(context.Background(),
		model.Request{Method: "", URL: "http://x/"}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.Method != http.MethodGet {
		t.Errorf("req.Method = %q, want GET", req.Method)
	}
	// Non-empty method passes through unchanged.
	req, _, err = Build(context.Background(),
		model.Request{Method: model.POST, URL: "http://x/"}, nil, nil)
	if err != nil {
		t.Fatalf("Build POST: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Errorf("req.Method = %q, want POST", req.Method)
	}
}

// TestBuildSetsContentLengthAndGetBodyForBodiedRequest verifies that
// when the request carries a body, Build populates ContentLength to
// len(body) and supplies a GetBody closure. Kills the NEGATION
// mutation on `if body != nil` that would otherwise leave both
// fields zero — net/http needs both for retries + redirects to
// work correctly on bodied requests.
func TestBuildSetsContentLengthAndGetBodyForBodiedRequest(t *testing.T) {
	body := `{"k":"v"}`
	req, _, err := Build(context.Background(),
		model.Request{Method: model.POST, URL: "http://x/",
			Body: model.Body{Type: model.BodyJSON, Content: body}}, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if req.ContentLength != int64(len(body)) {
		t.Errorf("ContentLength = %d, want %d", req.ContentLength, len(body))
	}
	if req.GetBody == nil {
		t.Error("GetBody not set on bodied request — retries won't have a body")
	}

	// Bodyless request: both fields should stay zero/nil.
	req, _, err = Build(context.Background(),
		model.Request{Method: model.GET, URL: "http://x/"}, nil, nil)
	if err != nil {
		t.Fatalf("Build GET: %v", err)
	}
	if req.ContentLength != 0 {
		t.Errorf("bodyless ContentLength = %d, want 0", req.ContentLength)
	}
	if req.GetBody != nil {
		t.Error("GetBody set on bodyless request")
	}
}

// TestDoCapturesResolvedRequestURLAndBody verifies that Response.RequestURL
// carries the resolved URL (vars substituted, query params merged) and
// that Response.RequestBody carries the encoded body bytes. These are
// the values chain.View surfaces as chain.<alias>.request.{url,body}
// — so scripts inspecting a chain step see what was actually sent, not
// the pre-resolution template.
func TestDoCapturesResolvedRequestURLAndBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.POST,
		URL:    "{{base}}/login",
		Params: []model.KeyValue{{Enabled: true, Key: "ts", Value: "{{ts}}"}},
		Body: model.Body{Type: model.BodyForm, Form: []model.KeyValue{
			{Enabled: true, Key: "user", Value: "{{u}}"},
			{Enabled: true, Key: "pw", Value: "{{p}}"},
		}},
	}
	res := vars.New(map[string]string{"base": ts.URL, "ts": "1234", "u": "alice", "p": "s3cret"})
	resp, err := c.Do(context.Background(), req, res)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !strings.HasPrefix(resp.RequestURL, ts.URL+"/login") {
		t.Errorf("RequestURL = %q, want prefix %q", resp.RequestURL, ts.URL+"/login")
	}
	if !strings.Contains(resp.RequestURL, "ts=1234") {
		t.Errorf("RequestURL = %q, want merged query ts=1234", resp.RequestURL)
	}
	got := string(resp.RequestBody)
	if !strings.Contains(got, "user=alice") || !strings.Contains(got, "pw=s3cret") {
		t.Errorf("RequestBody = %q, want URL-encoded form with user=alice & pw=s3cret", got)
	}
}
