package scripting

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// fakeBridge is a minimal EnvBridge that records every Set and lets
// tests preload Get values.
type fakeBridge struct {
	mu     sync.Mutex
	values map[string]string
	writes []write
}

type write struct{ name, value string }

func newFakeBridge() *fakeBridge {
	return &fakeBridge{values: map[string]string{}}
}

func (f *fakeBridge) Get(name string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[name]
	return v, ok
}

func (f *fakeBridge) Set(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[name] = value
	f.writes = append(f.writes, write{name, value})
}

// TestRunPreRequestEmpty verifies that an empty script is a no-op and
// returns an empty Result with no error.
func TestRunPreRequestEmpty(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	res, err := rt.RunPreRequest(context.Background(), "   \n  ", &r)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(res.Console) != 0 {
		t.Errorf("Console = %v, want empty", res.Console)
	}
}

// TestRunPreRequestMutatesScalars verifies that mutating request.method,
// url, and body in the script writes back into the model.
func TestRunPreRequestMutatesScalars(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/", Body: model.Body{Type: model.BodyText, Content: "old"}}
	src := `
		request.method = "POST";
		request.url = "https://y/";
		request.body = "new body";
	`
	_, err := rt.RunPreRequest(context.Background(), src, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.Method != "POST" {
		t.Errorf("Method = %q, want POST", r.Method)
	}
	if r.URL != "https://y/" {
		t.Errorf("URL = %q, want https://y/", r.URL)
	}
	if r.Body.Content != "new body" {
		t.Errorf("Body.Content = %q, want %q", r.Body.Content, "new body")
	}
}

// TestRunPreRequestHeadersAddUpdateDelete verifies the header merge
// rules: enabled rows update on JS-side change, new keys append as
// enabled, deleted keys are dropped, and disabled rows pass through
// untouched.
func TestRunPreRequestHeadersAddUpdateDelete(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{
		Method: model.GET, URL: "https://x/",
		Headers: []model.KeyValue{
			{Enabled: true, Key: "X-Update", Value: "old"},
			{Enabled: true, Key: "X-Delete", Value: "doomed"},
			{Enabled: false, Key: "X-Disabled", Value: "kept"},
		},
	}
	src := `
		request.headers["X-Update"] = "new";
		delete request.headers["X-Delete"];
		request.headers["X-New"] = "fresh";
	`
	_, err := rt.RunPreRequest(context.Background(), src, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := map[string]model.KeyValue{}
	for _, h := range r.Headers {
		got[h.Key] = h
	}
	if got["X-Update"].Value != "new" || !got["X-Update"].Enabled {
		t.Errorf("X-Update = %+v, want value=new enabled", got["X-Update"])
	}
	if _, ok := got["X-Delete"]; ok {
		t.Error("X-Delete should be removed")
	}
	if got["X-New"].Value != "fresh" || !got["X-New"].Enabled {
		t.Errorf("X-New = %+v, want value=fresh enabled", got["X-New"])
	}
	if got["X-Disabled"].Value != "kept" || got["X-Disabled"].Enabled {
		t.Errorf("X-Disabled = %+v, want value=kept disabled", got["X-Disabled"])
	}
}

// TestRunPreRequestParamsMerge mirrors the header merge but on Params.
func TestRunPreRequestParamsMerge(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{
		Method: model.GET, URL: "https://x/",
		Params: []model.KeyValue{{Enabled: true, Key: "page", Value: "1"}},
	}
	_, err := rt.RunPreRequest(context.Background(),
		`request.params["page"] = "2"; request.params["sort"] = "name";`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := map[string]string{}
	for _, p := range r.Params {
		got[p.Key] = p.Value
	}
	if got["page"] != "2" || got["sort"] != "name" {
		t.Errorf("Params = %v, want page=2 sort=name", got)
	}
}

// TestRunPreRequestHelenaEnvSet verifies a helena.env.set call lands in
// the bridge.
func TestRunPreRequestHelenaEnvSet(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(), `helena.env.set("TOKEN", "abc123");`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("TOKEN"); v != "abc123" {
		t.Errorf("bridge TOKEN = %q, want abc123", v)
	}
}

// TestRunPreRequestHelenaEnvGet verifies helena.env.get / helena.vars.get
// resolve through the bridge.
func TestRunPreRequestHelenaEnvGet(t *testing.T) {
	bridge := newFakeBridge()
	bridge.values["BASE"] = "https://api/"
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(),
		`request.url = helena.env.get("BASE") + "users";
		 request.headers["X-Alias"] = helena.vars.get("BASE");`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.URL != "https://api/users" {
		t.Errorf("URL = %q, want https://api/users", r.URL)
	}
	var alias string
	for _, h := range r.Headers {
		if h.Key == "X-Alias" {
			alias = h.Value
		}
	}
	if alias != "https://api/" {
		t.Errorf("X-Alias = %q, want https://api/", alias)
	}
}

// TestRunPostResponseReadsJSON verifies the canonical
// helena.env.set("TOKEN", response.json.token) flow against a real JSON
// payload.
func TestRunPostResponseReadsJSON(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	req := model.Request{Method: model.POST, URL: "https://x/login"}
	in := ResponseInput{
		StatusCode: 200, Status: "200 OK",
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"token":"abc","user":{"id":42}}`),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("TOKEN", response.json.token);
		 helena.env.set("UID", String(response.json.user.id));`, req, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("TOKEN"); v != "abc" {
		t.Errorf("TOKEN = %q, want abc", v)
	}
	if v, _ := bridge.Get("UID"); v != "42" {
		t.Errorf("UID = %q, want 42", v)
	}
}

// TestRunPostResponseReadsXML verifies XML body access via response.xml,
// covering text content under "_" and attribute access under "$".
func TestRunPostResponseReadsXML(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	req := model.Request{Method: model.GET, URL: "https://x/"}
	in := ResponseInput{
		StatusCode: 200, Status: "200 OK",
		Headers: http.Header{"Content-Type": []string{"application/xml"}},
		Body:    []byte(`<root id="1"><token>xyz</token></root>`),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("ID", response.xml.root.$.id);
		 helena.env.set("TOK", response.xml.root.token._);`, req, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("ID"); v != "1" {
		t.Errorf("ID = %q, want 1", v)
	}
	if v, _ := bridge.Get("TOK"); v != "xyz" {
		t.Errorf("TOK = %q, want xyz", v)
	}
}

// TestRunPostResponseHeadersCanonical verifies that response.headers
// canonicalizes keys to MIME form so case-insensitive access by
// canonical name works even when the input http.Header used a
// non-canonical case.
func TestRunPostResponseHeadersCanonical(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	in := ResponseInput{
		StatusCode: 200,
		Headers:    http.Header{"content-type": []string{"application/json"}},
		Body:       []byte(""),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("CT", response.headers["Content-Type"]);`, model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("CT"); v != "application/json" {
		t.Errorf("CT = %q, want application/json", v)
	}
}

// TestRunPostResponseStatusAndHeaders verifies the simple read-only
// surface: status int, statusText, and a header lookup.
func TestRunPostResponseStatusAndHeaders(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	in := ResponseInput{
		StatusCode: 201, Status: "201 Created",
		Headers: http.Header{"Location": []string{"/users/42"}},
		Body:    []byte(""),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("S", String(response.status));
		 helena.env.set("T", response.statusText);
		 helena.env.set("L", response.headers.Location);`,
		model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("S"); v != "201" {
		t.Errorf("S = %q, want 201", v)
	}
	if v, _ := bridge.Get("T"); v != "201 Created" {
		t.Errorf("T = %q, want '201 Created'", v)
	}
	if v, _ := bridge.Get("L"); v != "/users/42" {
		t.Errorf("L = %q, want /users/42", v)
	}
}

// TestConsoleCaptures verifies log / info / warn / error each produce a
// single Console line with the right prefix.
func TestConsoleCaptures(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	res, err := rt.RunPreRequest(context.Background(),
		`console.log("a", 1);
		 console.info("b");
		 console.warn("c");
		 console.error("d");`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"a 1", "b", "WARN: c", "ERROR: d"}
	if len(res.Console) != len(want) {
		t.Fatalf("Console = %v, want %v", res.Console, want)
	}
	for i := range want {
		if res.Console[i] != want[i] {
			t.Errorf("Console[%d] = %q, want %q", i, res.Console[i], want[i])
		}
	}
}

// TestConsoleStringifyObject verifies object args are JSON-encoded.
func TestConsoleStringifyObject(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	res, err := rt.RunPreRequest(context.Background(),
		`console.log({a: 1, b: "x"});`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(res.Console) != 1 {
		t.Fatalf("Console = %v, want 1 line", res.Console)
	}
	if !strings.Contains(res.Console[0], `"a":1`) || !strings.Contains(res.Console[0], `"b":"x"`) {
		t.Errorf("Console[0] = %q, expected JSON shape", res.Console[0])
	}
}

// TestScriptTimeout verifies an infinite loop is interrupted within
// ScriptTimeout. The test caps wall time at 2x the timeout to fail loud
// on a regression that would otherwise hang the suite.
func TestScriptTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in -short")
	}
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	done := make(chan error, 1)
	go func() {
		_, err := rt.RunPreRequest(context.Background(), `while(true){}`, &r)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("err = %v, want timed-out error", err)
		}
	case <-time.After(2 * ScriptTimeout):
		t.Fatal("script did not interrupt within 2x ScriptTimeout")
	}
}

// TestContextCancelled verifies ctx cancellation interrupts a running
// script promptly even before ScriptTimeout fires.
func TestContextCancelled(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := rt.RunPreRequest(ctx, `while(true){}`, &r)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("err = %v, want cancelled error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("script did not interrupt after ctx cancel")
	}
}

// TestScriptThrowSurfacesError verifies a user `throw` propagates as a
// Go error.
func TestScriptThrowSurfacesError(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(), `throw new Error("boom");`, &r)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want error containing boom", err)
	}
}

// TestNilEnvBridge verifies a nil bridge doesn't panic — helena.env.get
// returns "" and helena.env.set is a silent no-op.
func TestNilEnvBridge(t *testing.T) {
	rt := New(nil)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("X","Y"); request.url = helena.env.get("Z");`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.URL != "" {
		t.Errorf("URL = %q, want empty (helena.env.get on missing key)", r.URL)
	}
}

// TestRunPreRequestBadJSONResponseUndefined verifies a non-JSON,
// non-XML body leaves response.json and response.xml undefined.
func TestRunPostResponseUndefinedParsers(t *testing.T) {
	rt := New(newFakeBridge())
	in := ResponseInput{StatusCode: 200, Body: []byte("plain text")}
	res, err := rt.RunPostResponse(context.Background(),
		`console.log(typeof response.json, typeof response.xml);`,
		model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(res.Console) != 1 || res.Console[0] != "undefined undefined" {
		t.Errorf("Console = %v, want [undefined undefined]", res.Console)
	}
}

// TestRunPostResponseJSONArrayBody verifies a top-level JSON array
// parses to an indexable JS array.
func TestRunPostResponseJSONArrayBody(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	in := ResponseInput{
		StatusCode: 200,
		Body:       []byte(`[{"id":"a"},{"id":"b"}]`),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("FIRST", response.json[0].id);
		 helena.env.set("COUNT", String(response.json.length));`,
		model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("FIRST"); v != "a" {
		t.Errorf("FIRST = %q, want a", v)
	}
	if v, _ := bridge.Get("COUNT"); v != "2" {
		t.Errorf("COUNT = %q, want 2", v)
	}
}

// TestRunPreRequestFormBody verifies that for form-urlencoded /
// multipart bodies the script can read and mutate `request.form` and
// the merged result lands back on r.Body.Form. The flat `request.body`
// remains writable but Form is the canonical surface for structured
// fields, matching httpclient.buildBody's preference for Form when
// it's non-empty.
func TestRunPreRequestFormBody(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{
		Method: model.POST, URL: "https://x/",
		Body: model.Body{Type: model.BodyForm, Form: []model.KeyValue{
			{Enabled: true, Key: "user", Value: "old"},
		}},
	}
	_, err := rt.RunPreRequest(context.Background(),
		`request.form["user"] = "new"; request.form["token"] = "abc";`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := map[string]string{}
	for _, f := range r.Body.Form {
		got[f.Key] = f.Value
	}
	if got["user"] != "new" || got["token"] != "abc" {
		t.Errorf("Form = %v, want user=new token=abc", got)
	}
}

// TestRunPreRequestFormNotBoundForRawBodies verifies request.form is
// undefined for non-form body types so scripts can probe the body
// shape via `typeof request.form`.
func TestRunPreRequestFormNotBoundForRawBodies(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{
		Method: model.POST, URL: "https://x/",
		Body: model.Body{Type: model.BodyJSON, Content: "{}"},
	}
	bridge := newFakeBridge()
	rt = New(bridge)
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("HAS_FORM", typeof request.form);`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("HAS_FORM"); v != "undefined" {
		t.Errorf("typeof request.form = %q, want undefined", v)
	}
}

// TestMergeKVCaseInsensitiveNoDuplicate verifies that writing a header
// under a different case (`authorization` when the model has
// `Authorization`) updates the existing row in place rather than
// producing two case-variant rows with conflicting values.
func TestMergeKVCaseInsensitiveNoDuplicate(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{
		Method: model.GET, URL: "https://x/",
		Headers: []model.KeyValue{{Enabled: true, Key: "Authorization", Value: "Bearer old"}},
	}
	_, err := rt.RunPreRequest(context.Background(),
		`request.headers["authorization"] = "Bearer new";`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(r.Headers) != 1 {
		t.Fatalf("Headers count = %d, want 1\n%+v", len(r.Headers), r.Headers)
	}
	if r.Headers[0].Value != "Bearer new" {
		t.Errorf("Headers[0].Value = %q, want 'Bearer new'", r.Headers[0].Value)
	}
}

// TestMergeKVDuplicateCaseInJSObject verifies that when the script
// itself writes both `Content-Type` and `content-type` on the same JS
// object, dedup picks last-write-wins (matching JS object semantics) so
// the model ends up with one row, not two.
func TestMergeKVDuplicateCaseInJSObject(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(),
		`request.headers["Content-Type"] = "a"; request.headers["content-type"] = "b";`, &r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(r.Headers) != 1 {
		t.Fatalf("Headers count = %d, want 1\n%+v", len(r.Headers), r.Headers)
	}
	if r.Headers[0].Value != "b" {
		t.Errorf("Headers[0].Value = %q, want 'b'", r.Headers[0].Value)
	}
}

// TestThrowingToStringDoesNotPanic verifies that a script assigning a
// hostile object (whose toString throws) to a request property does NOT
// crash the runtime — writeBackRequest must turn the panic into a
// clean error and leave the model field at its original value.
func TestThrowingToStringDoesNotPanic(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://original/"}
	_, err := rt.RunPreRequest(context.Background(),
		`request.url = {toString(){ throw new Error("boom"); }};`, &r)
	if err == nil {
		// Either it returned an error (preferred) or it absorbed the
		// throw and left URL unchanged. Both are non-crash outcomes.
		if r.URL != "https://original/" {
			t.Errorf("URL = %q, want unchanged after absorbed throw", r.URL)
		}
	} else if r.URL != "https://original/" {
		t.Errorf("URL = %q after error, want unchanged", r.URL)
	}
}

// TestParseXMLDepthCap verifies that an XML body exceeding xmlMaxDepth
// is treated as not-XML — response.xml stays undefined, no stack
// overflow.
func TestParseXMLDepthCap(t *testing.T) {
	// Build <a><a><a>… 300 levels deep …</a></a></a>.
	const levels = 300
	var b strings.Builder
	for i := 0; i < levels; i++ {
		b.WriteString("<a>")
	}
	for i := 0; i < levels; i++ {
		b.WriteString("</a>")
	}
	bridge := newFakeBridge()
	rt := New(bridge)
	in := ResponseInput{Body: []byte(b.String())}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("X", typeof response.xml);`, model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("X"); v != "undefined" {
		t.Errorf("typeof response.xml = %q, want undefined (too deep)", v)
	}
}

// TestParseXMLMultipleChildren verifies same-named children coalesce
// into an array on the JS side.
func TestParseXMLMultipleChildren(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	in := ResponseInput{
		Body: []byte(`<root><item>1</item><item>2</item></root>`),
	}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("LEN", String(response.xml.root.item.length));
		 helena.env.set("FIRST", response.xml.root.item[0]._);`,
		model.Request{}, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("LEN"); v != "2" {
		t.Errorf("LEN = %q, want 2", v)
	}
	if v, _ := bridge.Get("FIRST"); v != "1" {
		t.Errorf("FIRST = %q, want 1", v)
	}
}
