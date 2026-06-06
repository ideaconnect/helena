package exporter

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestToCurlSimpleGET verifies a no-headers no-body GET renders as a single quoted-URL curl line.
func TestToCurlSimpleGET(t *testing.T) {
	got, err := ToCurl(
		model.Request{Method: model.GET, URL: "https://example.com/x"},
		nil, model.Settings{},
	)
	if err != nil {
		t.Fatalf("ToCurl: %v", err)
	}
	want := "curl -X GET 'https://example.com/x'"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestToCurlEmitsHostHeader verifies a custom Host header (which Go keeps on
// req.Host, out of req.Header) is still rendered, preserving vhost fidelity.
func TestToCurlEmitsHostHeader(t *testing.T) {
	r := model.Request{
		Method:  model.GET,
		URL:     "https://1.2.3.4/api",
		Headers: []model.KeyValue{{Enabled: true, Key: "Host", Value: "api.example.com"}},
	}
	got, err := ToCurl(r, nil, model.Settings{})
	if err != nil {
		t.Fatalf("ToCurl: %v", err)
	}
	if !strings.Contains(got, "Host: api.example.com") {
		t.Errorf("curl dropped the Host header:\n%s", got)
	}
}

// TestToCurlPOSTWithHeadersAndBody verifies that a JSON POST emits -H lines (including auto Content-Type) and --data-raw with quoted body.
func TestToCurlPOSTWithHeadersAndBody(t *testing.T) {
	r := model.Request{
		Method:  model.POST,
		URL:     "https://api.example/users",
		Headers: []model.KeyValue{{Enabled: true, Key: "X-Token", Value: "abc"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
	got, err := ToCurl(r, nil, model.Settings{})
	if err != nil {
		t.Fatalf("ToCurl: %v", err)
	}
	want := strings.Join([]string{
		"curl -X POST 'https://api.example/users'",
		"  -H 'Content-Type: application/json'",
		"  -H 'X-Token: abc'",
		"  --data-raw '{\"a\":1}'",
	}, " \\\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestToCurlResolvesVarsAndParams verifies that {{vars}} get resolved into the URL and disabled params are stripped before rendering.
func TestToCurlResolvesVarsAndParams(t *testing.T) {
	r := model.Request{
		Method: model.GET,
		URL:    "{{base}}/users",
		Params: []model.KeyValue{
			{Enabled: true, Key: "q", Value: "{{q}}"},
			{Enabled: false, Key: "skip", Value: "no"},
		},
	}
	res := vars.New(map[string]string{
		"base": "https://example.com",
		"q":    "hello world",
	})
	got, err := ToCurl(r, res, model.Settings{})
	if err != nil {
		t.Fatalf("ToCurl: %v", err)
	}
	// The URL with the resolved query string should be present; the disabled
	// param should not.
	if !strings.Contains(got, "https://example.com/users?q=hello+world") {
		t.Errorf("expected resolved URL with encoded query in:\n%s", got)
	}
	if strings.Contains(got, "skip") {
		t.Errorf("disabled param leaked into output:\n%s", got)
	}
}

// TestToCurlSettingsFlags verifies that InsecureSkipVerify, FollowRedirects and TimeoutSeconds translate to -k, -L and --max-time.
func TestToCurlSettingsFlags(t *testing.T) {
	r := model.Request{Method: model.GET, URL: "https://x"}
	got, _ := ToCurl(r, nil, model.Settings{
		InsecureSkipVerify: true,
		FollowRedirects:    true,
		TimeoutSeconds:     30,
	})
	for _, want := range []string{"-k", "-L", "--max-time 30"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing flag %q in:\n%s", want, got)
		}
	}
}

// TestToWgetPOST verifies that wget rendering uses --method, --header, --body-data and the --no-check-certificate / --timeout flags.
func TestToWgetPOST(t *testing.T) {
	r := model.Request{
		Method:  model.POST,
		URL:     "https://api.example/users",
		Headers: []model.KeyValue{{Enabled: true, Key: "X-Token", Value: "abc"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
	got, err := ToWget(r, nil, model.Settings{InsecureSkipVerify: true, TimeoutSeconds: 30})
	if err != nil {
		t.Fatalf("ToWget: %v", err)
	}
	for _, want := range []string{
		"wget --method=POST",
		"--no-check-certificate",
		"--timeout=30",
		"--header='Content-Type: application/json'",
		"--header='X-Token: abc'",
		"--body-data='{\"a\":1}'",
		"-qO-",
		"'https://api.example/users'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestToWgetNoFollowRedirectsAddsFlag verifies the inverted wget semantics: --max-redirect=0 only when FollowRedirects is false.
func TestToWgetNoFollowRedirectsAddsFlag(t *testing.T) {
	got, _ := ToWget(
		model.Request{Method: model.GET, URL: "https://x"},
		nil, model.Settings{FollowRedirects: false},
	)
	if !strings.Contains(got, "--max-redirect=0") {
		t.Errorf("expected --max-redirect=0 in:\n%s", got)
	}
}

// TestUnresolvedVarsReturnsError verifies that unresolved {{vars}} surface as errors from ToCurl via the underlying Build call.
func TestUnresolvedVarsReturnsError(t *testing.T) {
	_, err := ToCurl(
		model.Request{Method: model.GET, URL: "{{base}}/x"},
		nil, model.Settings{},
	)
	if err == nil {
		t.Fatalf("expected error for unresolved {{base}}")
	}
}

// TestShellQuoteEdgeCases verifies that shell-safe strings stay bare, empty / spaced / quoted strings get single-quoted, and inner quotes use the '\” trick.
func TestShellQuoteEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                  "''",
		"simple":            "simple",
		"with space":        "'with space'",
		"single'quote":      `'single'\''quote'`,
		"back\\slash":       `'back\slash'`,
		"https://x?y=1":     "'https://x?y=1'",
		"path/with/slashes": "path/with/slashes",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
