package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestDoSubstitutesPathParamOnWire verifies the full send path (client.Do)
// puts the filled {name} on the wire in the URL PATH — not the query string —
// and leaves an unfilled token literal, exercising the real HTTP round-trip
// rather than just Build's assembled URL.
func TestDoSubstitutesPathParamOnWire(t *testing.T) {
	var gotPath, gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := New(model.DefaultSettings())
	req := model.Request{
		Method:     model.GET,
		URL:        "{{base}}/bag/{bagId}/leaf/{missing}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "42"}},
		Params:     []model.KeyValue{{Enabled: true, Key: "q", Value: "hi"}},
	}
	if _, err := c.Do(context.Background(), req, vars.New(map[string]string{"base": ts.URL})); err != nil {
		t.Fatalf("Do: %v", err)
	}
	// The server decodes %7Bmissing%7D back to {missing} in r.URL.Path.
	if gotPath != "/bag/42/leaf/{missing}" {
		t.Errorf("wire path = %q, want /bag/42/leaf/{missing}", gotPath)
	}
	if gotQuery != "q=hi" {
		t.Errorf("wire query = %q, want q=hi (path param must not leak into the query)", gotQuery)
	}
}

// buildURL runs Build and returns the assembled request URL, failing on error.
func buildURL(t *testing.T, r model.Request, res *vars.Resolver) string {
	t.Helper()
	req, _, err := Build(context.Background(), r, res, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return req.URL.String()
}

// TestBuildFillsPathParams verifies enabled {name} tokens are replaced with
// their values after {{var}} resolution, and that query params still append.
func TestBuildFillsPathParams(t *testing.T) {
	res := vars.New(map[string]string{"base_url": "https://api.example.com"})
	got := buildURL(t, model.Request{
		Method:     model.GET,
		URL:        "{{base_url}}/drs/bag/{bagId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "42"}},
		Params:     []model.KeyValue{{Enabled: true, Key: "q", Value: "x"}},
	}, res)
	if got != "https://api.example.com/drs/bag/42?q=x" {
		t.Errorf("URL = %q", got)
	}
}

// TestBuildLeavesUnfilledPathParamLiteral verifies a missing or empty-valued
// path parameter reaches the wire as a literal {name} rather than an empty
// segment, and does not error the send.
func TestBuildLeavesUnfilledPathParamLiteral(t *testing.T) {
	got := buildURL(t, model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/bag/{bagId}/x/{other}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: ""}},
	}, vars.New())
	if got != "https://api.example.com/bag/%7BbagId%7D/x/%7Bother%7D" {
		t.Errorf("URL = %q, want the {bagId}/{other} tokens preserved (percent-encoded by net/url)", got)
	}
}

// TestBuildResolvesVarsInsidePathParamValue verifies a path parameter value may
// itself reference a {{variable}}.
func TestBuildResolvesVarsInsidePathParamValue(t *testing.T) {
	res := vars.New(map[string]string{"defaultBag": "abc"})
	got := buildURL(t, model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/bag/{bagId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "{{defaultBag}}"}},
	}, res)
	if got != "https://api.example.com/bag/abc" {
		t.Errorf("URL = %q", got)
	}
}

// TestBuildReportsUnresolvedVarInPathParamValue verifies an unresolved {{var}}
// inside a path-parameter value surfaces in the missing-variables error, just
// like one in the URL or a header.
func TestBuildReportsUnresolvedVarInPathParamValue(t *testing.T) {
	_, _, err := Build(context.Background(), model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/bag/{bagId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "{{NOPE}}"}},
	}, vars.New(), nil)
	if err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("Build err = %v, want one mentioning NOPE", err)
	}
}

// TestBuildDisabledPathParamLeftLiteral verifies a disabled path parameter is
// not substituted; its token is left literal.
func TestBuildDisabledPathParamLeftLiteral(t *testing.T) {
	got := buildURL(t, model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/bag/{bagId}",
		PathParams: []model.KeyValue{{Enabled: false, Key: "bagId", Value: "42"}},
	}, vars.New())
	if !strings.Contains(got, "%7BbagId%7D") {
		t.Errorf("URL = %q, want the disabled param left as a literal token", got)
	}
}

// TestBuildDoesNotTouchTemplateInnerName verifies a {{variable}} whose name
// matches a path parameter is resolved as a variable, not filled as a path
// parameter — the two syntaxes never cross.
func TestBuildDoesNotTouchTemplateInnerName(t *testing.T) {
	res := vars.New(map[string]string{"bagId": "from-var"})
	got := buildURL(t, model.Request{
		Method:     model.GET,
		URL:        "https://api.example.com/{{bagId}}/{bagId}",
		PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "from-path"}},
	}, res)
	if got != "https://api.example.com/from-var/from-path" {
		t.Errorf("URL = %q", got)
	}
}
