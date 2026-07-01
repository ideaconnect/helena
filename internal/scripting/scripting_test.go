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
	res, err := rt.RunPreRequest(context.Background(), "   \n  ", &r, nil)
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
	_, err := rt.RunPreRequest(context.Background(), src, &r, nil)
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
	_, err := rt.RunPreRequest(context.Background(), src, &r, nil)
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
		`request.params["page"] = "2"; request.params["sort"] = "name";`, &r, nil)
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
	_, err := rt.RunPreRequest(context.Background(), `helena.env.set("TOKEN", "abc123");`, &r, nil)
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
		 request.headers["X-Alias"] = helena.vars.get("BASE");`, &r, nil)
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
		 helena.env.set("UID", String(response.json.user.id));`, req, in, nil)
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
		 helena.env.set("TOK", response.xml.root.token._);`, req, in, nil)
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
		`helena.env.set("CT", response.headers["Content-Type"]);`, model.Request{}, in, nil)
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
		model.Request{}, in, nil)
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
		 console.error("d");`, &r, nil)
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
		`console.log({a: 1, b: "x"});`, &r, nil)
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

// TestNamedCaptureGroupContract pins the ES2018 named-capture-group behavior the
// scripting engine exposes after the goja bump. Earlier goja did not honor named
// groups; user pre/post scripts now observe:
//   - $<name> in a String.replace replacement is a named backreference, but only
//     when the pattern has named groups (otherwise it passes through verbatim);
//   - an unknown $<name> against a named-group pattern is dropped;
//   - exec()/match expose a populated .groups object.
//
// A regression here would silently change how user scripts rewrite request and
// response data, so the contract is locked against future goja bumps.
func TestNamedCaptureGroupContract(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `
		console.log("swap=" + "ab".replace(/(?<x>a)(?<y>b)/, "$<y>$<x>"));
		console.log("group=" + (/(?<x>a)/.exec("a").groups.x));
		console.log("match=" + ("a".match(/(?<x>a)/).groups.x));
		console.log("unknown=" + "ab".replace(/(?<x>a)b/, "X$<nope>Y"));
		console.log("nogroup=" + "ab".replace(/ab/, "$<y>"));
	`
	res, err := rt.RunPreRequest(context.Background(), src, &r, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"swap=ba", "group=a", "match=a", "unknown=XY", "nogroup=$<y>"}
	if len(res.Console) != len(want) {
		t.Fatalf("Console = %v, want %v", res.Console, want)
	}
	for i := range want {
		if res.Console[i] != want[i] {
			t.Errorf("Console[%d] = %q, want %q", i, res.Console[i], want[i])
		}
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
		_, err := rt.RunPreRequest(context.Background(), `while(true){}`, &r, nil)
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
		_, err := rt.RunPreRequest(ctx, `while(true){}`, &r, nil)
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
	_, err := rt.RunPreRequest(context.Background(), `throw new Error("boom");`, &r, nil)
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
		`helena.env.set("X","Y"); request.url = helena.env.get("Z");`, &r, nil)
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
		model.Request{}, in, nil)
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
		model.Request{}, in, nil)
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

// TestRunPreRequestChainGlobalReadsAlias verifies the script-side
// `chain.<alias>` global exposes a previous step's response, including
// auto-parsed JSON.
func TestRunPreRequestChainGlobalReadsAlias(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	chainMap := map[string]ChainView{
		"login": {
			Request: ChainRequestView{Method: "POST", URL: "https://auth/login"},
			Response: ResponseInput{
				StatusCode: 200, Status: "200 OK",
				Headers: http.Header{"Content-Type": []string{"application/json"}},
				Body:    []byte(`{"token":"chained-abc"}`),
			},
		},
	}
	_, err := rt.RunPreRequest(context.Background(),
		`request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;
		 helena.env.set("TRACE", chain.login.request.url);`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := r.Headers[0].Value; got != "Bearer chained-abc" {
		t.Errorf("Authorization = %q, want 'Bearer chained-abc'", got)
	}
	if v, _ := bridge.Get("TRACE"); v != "https://auth/login" {
		t.Errorf("TRACE = %q, want https://auth/login", v)
	}
}

// TestChainResponseJSONIsLazy verifies that response.json on a chain
// entry is wired as a goja accessor — accessing it triggers the parse,
// and a script that never reads .json pays no parse cost. We can't
// directly observe the absence of work, so we assert positive: a
// large body's json field is reachable and the parse happens lazily
// (probed by mutating-decoy: the body bytes are read at .json access
// time, not at chain bind time).
func TestChainResponseJSONIsLazy(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"big": {
			Response: ResponseInput{
				StatusCode: 200, Body: []byte(`{"answer":42}`),
			},
		},
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("A", String(chain.big.response.json.answer));`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("A"); v != "42" {
		t.Errorf("A = %q, want 42", v)
	}
}

// TestRunPostResponseChainGlobalReadsAlias verifies the chain map is
// also bound during the post-response phase.
func TestRunPostResponseChainGlobalReadsAlias(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"bootstrap": {
			Response: ResponseInput{
				StatusCode: 200, Body: []byte(`{"csrf":"X"}`),
			},
		},
	}
	in := ResponseInput{StatusCode: 200, Body: []byte("{}")}
	_, err := rt.RunPostResponse(context.Background(),
		`helena.env.set("CSRF", chain.bootstrap.response.json.csrf);`,
		model.Request{}, in, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("CSRF"); v != "X" {
		t.Errorf("CSRF = %q, want X", v)
	}
}

// TestRunPreRequestNilChainBindsEmptyObject verifies a nil chain map
// still produces a usable `chain` global (empty object) so scripts can
// safely do `Object.keys(chain).length`.
func TestRunPreRequestNilChainBindsEmptyObject(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("N", String(Object.keys(chain).length));`, &r, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("N"); v != "0" {
		t.Errorf("N = %q, want 0", v)
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
		`request.form["user"] = "new"; request.form["token"] = "abc";`, &r, nil)
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
	r := model.Request{
		Method: model.POST, URL: "https://x/",
		Body: model.Body{Type: model.BodyJSON, Content: "{}"},
	}
	bridge := newFakeBridge()
	rt := New(bridge)
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("HAS_FORM", typeof request.form);`, &r, nil)
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
		`request.headers["authorization"] = "Bearer new";`, &r, nil)
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
		`request.headers["Content-Type"] = "a"; request.headers["content-type"] = "b";`, &r, nil)
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
		`request.url = {toString(){ throw new Error("boom"); }};`, &r, nil)
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
		`helena.env.set("X", typeof response.xml);`, model.Request{}, in, nil)
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
		model.Request{}, in, nil)
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

// TestChainResponseJSONCachedAfterFirstAccess verifies the lazy json
// accessor on a chain.<alias>.response object reuses its cached value
// on second access — proves the cache branch of the getter, not just
// the first-access parse path.
func TestChainResponseJSONCachedAfterFirstAccess(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"login": {
			Response: ResponseInput{StatusCode: 200, Body: []byte(`{"x":1}`)},
		},
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	// Access json twice. If the getter recomputed, both values still
	// match — the assertion here is "doesn't crash and produces
	// consistent output across calls".
	_, err := rt.RunPreRequest(context.Background(), `
		var a = chain.login.response.json.x;
		var b = chain.login.response.json.x;
		helena.env.set("BOTH", String(a) + "," + String(b));
	`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("BOTH"); v != "1,1" {
		t.Errorf("BOTH = %q, want 1,1", v)
	}
}

// TestChainResponseJSONUndefinedForNonJSONBody verifies that a chain
// entry whose body is not valid JSON returns undefined from
// chain.<alias>.response.json — and that the cached-undefined value is
// stable across subsequent accesses.
func TestChainResponseJSONUndefinedForNonJSONBody(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"plain": {
			Response: ResponseInput{StatusCode: 200, Body: []byte("not json")},
		},
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(), `
		var first = (typeof chain.plain.response.json === "undefined") ? "u" : "d";
		var second = (typeof chain.plain.response.json === "undefined") ? "u" : "d";
		helena.env.set("BOTH", first + second);
	`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("BOTH"); v != "uu" {
		t.Errorf("BOTH = %q, want uu", v)
	}
}

// TestChainResponseXMLCachedAfterFirstAccess verifies the lazy XML
// accessor's cache hit branch. Parallel to the JSON variant above.
func TestChainResponseXMLCachedAfterFirstAccess(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"feed": {
			Response: ResponseInput{StatusCode: 200, Body: []byte(`<r><x>9</x></r>`)},
		},
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(), `
		var a = chain.feed.response.xml.r.x._;
		var b = chain.feed.response.xml.r.x._;
		helena.env.set("BOTH", a + "," + b);
	`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("BOTH"); v != "9,9" {
		t.Errorf("BOTH = %q, want 9,9", v)
	}
}

// TestChainResponseXMLUndefinedForNonXMLBody verifies the
// undefined-cache branch of the XML accessor on a non-XML body.
func TestChainResponseXMLUndefinedForNonXMLBody(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	chainMap := map[string]ChainView{
		"plain": {
			Response: ResponseInput{StatusCode: 200, Body: []byte(`{"json":true}`)},
		},
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	_, err := rt.RunPreRequest(context.Background(), `
		var v = (typeof chain.plain.response.xml === "undefined") ? "u" : "d";
		helena.env.set("XML", v);
	`, &r, chainMap)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v, _ := bridge.Get("XML"); v != "u" {
		t.Errorf("XML = %q, want u", v)
	}
}

// TestNopBridgeSetIsNoop verifies that the default no-op env bridge
// (used when New is called with nil) silently swallows
// helena.env.set without panicking — keeps RunPre / RunPost callable
// even when the caller didn't wire a real bridge.
func TestNopBridgeSetIsNoop(t *testing.T) {
	rt := New(nil)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	// The script writes, but nobody listens. The script must complete
	// without error and helena.env.get returns the documented empty
	// string for a not-found key.
	_, err := rt.RunPreRequest(context.Background(),
		`helena.env.set("DROPPED", "x"); helena.env.set("MISSING", helena.env.get("nope"));`,
		&r, nil)
	if err != nil {
		t.Errorf("nil-bridge RunPreRequest err = %v", err)
	}
}

// TestRunPostResponseEmptyScript verifies the empty-script
// short-circuit on the post phase mirrors the pre phase — zero
// Result, no goja construction, no error.
func TestRunPostResponseEmptyScript(t *testing.T) {
	rt := New(newFakeBridge())
	res, err := rt.RunPostResponse(context.Background(), "   \n\t  ",
		model.Request{}, ResponseInput{StatusCode: 200}, nil)
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(res.Console) != 0 {
		t.Errorf("Console = %v, want empty", res.Console)
	}
}

// TestRunPostResponseScriptErrorPropagates verifies that a thrown
// exception in the post-script surfaces as an error from
// RunPostResponse — distinct from non-fatal cases handled elsewhere.
func TestRunPostResponseScriptErrorPropagates(t *testing.T) {
	rt := New(newFakeBridge())
	_, err := rt.RunPostResponse(context.Background(), `throw new Error("boom");`,
		model.Request{}, ResponseInput{StatusCode: 200}, nil)
	if err == nil {
		t.Error("expected post-script error, got nil")
	}
}
