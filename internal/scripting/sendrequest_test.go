package scripting

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestSendSpecToRequest covers the SendSpec -> model.Request conversion: method
// defaults to GET, headers are sorted, a body becomes a text body (#92).
func TestSendSpecToRequest(t *testing.T) {
	got := SendSpec{URL: "https://x/", Headers: map[string]string{"B": "2", "A": "1"}, Body: "hi"}.ToRequest()
	if got.Method != model.GET {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if len(got.Headers) != 2 || got.Headers[0].Key != "A" || got.Headers[1].Key != "B" {
		t.Errorf("headers not sorted: %+v", got.Headers)
	}
	if got.Body.Type != model.BodyText || got.Body.Content != "hi" {
		t.Errorf("body = %+v, want text 'hi'", got.Body)
	}
	// Empty body stays BodyNone (zero value).
	if (SendSpec{URL: "https://x/", Method: "post"}).ToRequest().Body.Type != "" {
		t.Error("empty body should not set a body type")
	}
}

// TestSendRequestInjected verifies helena.sendRequest passes the parsed spec to
// the injected requester and exposes the response (status/json) to the script.
func TestSendRequestInjected(t *testing.T) {
	var gotSpec SendSpec
	requester := func(spec SendSpec) (ResponseInput, error) {
		gotSpec = spec
		return ResponseInput{StatusCode: 201, Status: "201 Created",
			Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"id":7}`)}, nil
	}
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `
		var resp = helena.sendRequest({ method: "post", url: "https://api/u", headers: { "X-A": "1" }, body: "payload" });
		helena.env.set("status", String(resp.status));
		helena.env.set("id", String(resp.json.id));`
	bridge := newFakeBridge()
	rt := New(bridge)
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithRequester(requester)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if gotSpec.Method != "POST" || gotSpec.URL != "https://api/u" || gotSpec.Headers["X-A"] != "1" || gotSpec.Body != "payload" {
		t.Errorf("requester got spec %+v", gotSpec)
	}
	if v, _ := bridge.Get("status"); v != "201" {
		t.Errorf("status = %q, want 201", v)
	}
	if v, _ := bridge.Get("id"); v != "7" {
		t.Errorf("json.id = %q, want 7", v)
	}
}

// TestSendRequestUnavailableThrows verifies sendRequest throws (catchably) when
// no requester is wired — it has no meaning outside a Send.
func TestSendRequestUnavailableThrows(t *testing.T) {
	r := model.Request{Method: model.GET, URL: "https://x/"}
	bridge := newFakeBridge()
	rt := New(bridge)
	src := `helena.env.set("out", (function(){ try { helena.sendRequest({url:"https://x/"}); return "no-throw"; } catch(e){ return "threw"; } })());`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("out"); v != "threw" {
		t.Errorf("sendRequest without requester = %q, want threw", v)
	}
}

// TestSendRequestErrorAndBadSpec verifies a requester error and an invalid spec
// both surface as catchable throws.
func TestSendRequestErrorAndBadSpec(t *testing.T) {
	requester := func(SendSpec) (ResponseInput, error) { return ResponseInput{}, errors.New("boom") }
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `
		helena.env.set("err", (function(){ try { helena.sendRequest({url:"https://x/"}); return "no"; } catch(e){ return "threw"; } })());
		helena.env.set("nourl", (function(){ try { helena.sendRequest({}); return "no"; } catch(e){ return "threw"; } })());`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithRequester(requester)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("err"); v != "threw" {
		t.Errorf("requester error = %q, want threw", v)
	}
	if v, _ := bridge.Get("nourl"); v != "threw" {
		t.Errorf("missing url = %q, want threw", v)
	}
}
