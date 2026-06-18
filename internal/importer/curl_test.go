package importer

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

func kv(k, v string) model.KeyValue { return model.KeyValue{Enabled: true, Key: k, Value: v} }

func hdr(t *testing.T, req model.Request, key string) (string, bool) {
	t.Helper()
	for _, h := range req.Headers {
		if h.Key == key {
			return h.Value, true
		}
	}
	return "", false
}

func TestFromCurlSimpleGet(t *testing.T) {
	req, err := FromCurl("curl https://api.test/things")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.GET {
		t.Errorf("method = %q; want GET", req.Method)
	}
	if req.URL != "https://api.test/things" {
		t.Errorf("url = %q", req.URL)
	}
	if req.Body.Type != model.BodyNone {
		t.Errorf("body type = %q; want none", req.Body.Type)
	}
}

func TestFromCurlPostJSONWithHeaders(t *testing.T) {
	cmd := `curl -X POST https://api.test/v1/users \
	  -H 'Content-Type: application/json; charset=utf-8' \
	  -H "Authorization: Bearer abc123" \
	  --data-raw '{"name":"Ada","age":36}'`
	req, err := FromCurl(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.POST {
		t.Errorf("method = %q; want POST", req.Method)
	}
	if req.URL != "https://api.test/v1/users" {
		t.Errorf("url = %q", req.URL)
	}
	if req.Body.Type != model.BodyJSON || req.Body.Content != `{"name":"Ada","age":36}` {
		t.Errorf("body = %+v; want JSON with raw content", req.Body)
	}
	if v, ok := hdr(t, req, "Authorization"); !ok || v != "Bearer abc123" {
		t.Errorf("Authorization header = %q (ok=%v)", v, ok)
	}
	if v, _ := hdr(t, req, "Content-Type"); v != "application/json; charset=utf-8" {
		t.Errorf("Content-Type header = %q", v)
	}
}

func TestFromCurlRepeatedHeaders(t *testing.T) {
	req, err := FromCurl(`curl -H "X-A: 1" -H "X-B: 2" -H "X-A: 3" https://api.test/`)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Headers) != 3 {
		t.Fatalf("want 3 header rows, got %d (%+v)", len(req.Headers), req.Headers)
	}
}

func TestFromCurlDataDefaultsToFormAndPOST(t *testing.T) {
	req, err := FromCurl(`curl https://api.test/login -d 'user=ada&pw=secret%20x'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.POST {
		t.Errorf("method = %q; want POST (implied by -d)", req.Method)
	}
	if req.Body.Type != model.BodyForm {
		t.Fatalf("body type = %q; want form-urlencoded", req.Body.Type)
	}
	want := []model.KeyValue{kv("user", "ada"), kv("pw", "secret x")}
	if len(req.Body.Form) != 2 || req.Body.Form[0] != want[0] || req.Body.Form[1] != want[1] {
		t.Errorf("form = %+v; want %+v (percent-decoded)", req.Body.Form, want)
	}
}

func TestFromCurlMultipleDataJoined(t *testing.T) {
	req, err := FromCurl(`curl https://api.test/ -d a=1 -d b=2`)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Body.Form) != 2 {
		t.Errorf("multiple -d should join with &: %+v", req.Body.Form)
	}
}

func TestFromCurlMultipartForm(t *testing.T) {
	req, err := FromCurl(`curl https://api.test/upload -F field=value -F file=@/tmp/x.png`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.POST || req.Body.Type != model.BodyMultipart {
		t.Fatalf("want POST multipart, got %s %s", req.Method, req.Body.Type)
	}
	if len(req.Body.Form) != 2 || req.Body.Form[1].Value != "@/tmp/x.png" {
		t.Errorf("form = %+v", req.Body.Form)
	}
}

func TestFromCurlBasicAuth(t *testing.T) {
	req, err := FromCurl(`curl -u ada:secret https://api.test/`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Auth.Type != model.AuthBasic || req.Auth.Basic == nil {
		t.Fatalf("auth = %+v; want basic", req.Auth)
	}
	if req.Auth.Basic.Username != "ada" || req.Auth.Basic.Password != "secret" {
		t.Errorf("basic = %+v", req.Auth.Basic)
	}
}

func TestFromCurlGetModeMovesDataToQuery(t *testing.T) {
	req, err := FromCurl(`curl -G https://api.test/search -d q=cats -d page=2`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.GET {
		t.Errorf("method = %q; want GET", req.Method)
	}
	if req.URL != "https://api.test/search?q=cats&page=2" {
		t.Errorf("url = %q; want data folded into query", req.URL)
	}
	if req.Body.Type != model.BodyNone {
		t.Errorf("body type = %q; -G should not set a body", req.Body.Type)
	}
}

func TestFromCurlConvenienceHeaders(t *testing.T) {
	req, err := FromCurl(`curl https://api.test/ -A 'Mozilla/5' -e https://ref.test -b 'sid=42'`)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"User-Agent": "Mozilla/5", "Referer": "https://ref.test", "Cookie": "sid=42"} {
		if v, ok := hdr(t, req, k); !ok || v != want {
			t.Errorf("%s = %q (ok=%v); want %q", k, v, ok, want)
		}
	}
}

func TestFromCurlAttachedAndInlineFlags(t *testing.T) {
	req, err := FromCurl(`curl -XPOST --url=https://api.test/x --header=Accept:application/json`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != model.POST {
		t.Errorf("method = %q; want POST (-XPOST)", req.Method)
	}
	if req.URL != "https://api.test/x" {
		t.Errorf("url = %q; want from --url=", req.URL)
	}
	if v, ok := hdr(t, req, "Accept"); !ok || v != "application/json" {
		t.Errorf("Accept = %q (ok=%v)", v, ok)
	}
}

func TestFromCurlIgnoresNoiseFlags(t *testing.T) {
	// --compressed (boolean) and -o file (value) must not swallow the URL.
	req, err := FromCurl(`curl --compressed -s -o /dev/null https://api.test/x`)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://api.test/x" {
		t.Errorf("url = %q; noise flags derailed the parse", req.URL)
	}
}

func TestFromCurlNoURLErrors(t *testing.T) {
	if _, err := FromCurl(`curl -X POST -H 'X: 1'`); err == nil {
		t.Error("expected an error when no URL is present")
	}
}

func TestFromCurlDataRawJSONSniff(t *testing.T) {
	// --data-raw with no Content-Type but JSON-shaped → JSON body.
	req, err := FromCurl(`curl https://api.test/ --data-raw '[1,2,3]'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Body.Type != model.BodyJSON || req.Body.Content != "[1,2,3]" {
		t.Errorf("body = %+v; want sniffed JSON", req.Body)
	}
}

func TestFromCurlPlainTextDataSniff(t *testing.T) {
	req, err := FromCurl(`curl https://api.test/ -d 'hello world'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Body.Type != model.BodyText || req.Body.Content != "hello world" {
		t.Errorf("body = %+v; want text", req.Body)
	}
}

func TestTokenizeShellQuotingAndContinuations(t *testing.T) {
	toks, err := tokenizeShell("curl 'a b' \"c\\\"d\" e\\ f \\\n g")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"curl", "a b", `c"d`, "e f", "g"}
	if len(toks) != len(want) {
		t.Fatalf("tokens = %#v; want %#v", toks, want)
	}
	for i := range want {
		if toks[i] != want[i] {
			t.Errorf("token[%d] = %q; want %q", i, toks[i], want[i])
		}
	}
}

func TestTokenizeShellUnterminatedQuote(t *testing.T) {
	if _, err := tokenizeShell(`curl 'oops`); err == nil {
		t.Error("expected error for unterminated quote")
	}
}

func TestFromCurlName(t *testing.T) {
	req, _ := FromCurl(`curl https://api.test/v1/users/`)
	if req.Name != "GET api.test/v1/users" {
		t.Errorf("name = %q; want %q", req.Name, "GET api.test/v1/users")
	}
}

func TestFromCurlExplicitContentTypes(t *testing.T) {
	cases := []struct {
		ct   string
		data string
		want model.BodyType
	}{
		{"application/xml", "<a/>", model.BodyXML},
		{"application/x-www-form-urlencoded", "a=1&b=2", model.BodyForm},
		{"text/plain", "{not json}", model.BodyText},
		{"application/octet-stream", "raw", model.BodyText},
	}
	for _, c := range cases {
		req, err := FromCurl(`curl https://api.test/ -H 'Content-Type: ` + c.ct + `' -d '` + c.data + `'`)
		if err != nil {
			t.Fatal(err)
		}
		if req.Body.Type != c.want {
			t.Errorf("ct %q: body type = %q; want %q", c.ct, req.Body.Type, c.want)
		}
	}
}

func TestFromCurlGetModeAppendsToExistingQuery(t *testing.T) {
	req, err := FromCurl(`curl -G 'https://api.test/search?x=1' -d q=cats`)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://api.test/search?x=1&q=cats" {
		t.Errorf("url = %q; want existing query extended with &", req.URL)
	}
}

func TestParseFormDataEdgeCases(t *testing.T) {
	got := parseFormData("a=1&&=skip&b=2&keyonly")
	want := []model.KeyValue{kv("a", "1"), kv("b", "2"), kv("keyonly", "")}
	if len(got) != len(want) {
		t.Fatalf("parseFormData = %+v; want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v; want %+v", i, got[i], want[i])
		}
	}
}

func TestTokenizeShellDoubleQuoteEscapes(t *testing.T) {
	toks, err := tokenizeShell(`x "a\\b\$c\` + "`" + `d" 'lit\n'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x", `a\b$c` + "`" + `d`, `lit\n`}
	if len(toks) != len(want) {
		t.Fatalf("tokens = %#v; want %#v", toks, want)
	}
	for i := range want {
		if toks[i] != want[i] {
			t.Errorf("token[%d] = %q; want %q", i, toks[i], want[i])
		}
	}
}

func TestFromCurlEmptyIsError(t *testing.T) {
	if _, err := FromCurl("   "); err == nil {
		t.Error("expected error for empty command (no URL)")
	}
}

func TestFromCurlDataUrlencodeEncodes(t *testing.T) {
	// curl percent-encodes the content of --data-urlencode, keeping the name.
	req, err := FromCurl(`curl -G https://api.test/s --data-urlencode 'q=a b&c'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://api.test/s?q=a+b%26c" {
		t.Errorf("url = %q; want the --data-urlencode content percent-encoded", req.URL)
	}
}

func TestFromCurlMultipartDropsStaleContentType(t *testing.T) {
	// A pasted multipart Content-Type carries a boundary the send path can't
	// reuse, so it must be dropped (httpclient regenerates it).
	req, err := FromCurl(`curl https://api.test/u -H 'Content-Type: multipart/form-data; boundary=XYZ' -F a=1`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Body.Type != model.BodyMultipart {
		t.Fatalf("body type = %q; want multipart", req.Body.Type)
	}
	for _, h := range req.Headers {
		if strings.EqualFold(h.Key, "Content-Type") {
			t.Errorf("stale multipart Content-Type header should be dropped, got %q", h.Value)
		}
	}
}
