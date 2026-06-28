package scripting

import (
	"context"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestCookiesGetAndGetAll verifies helena.cookies.get/getAll read through the
// injected lookup, return undefined for a missing name, and an empty object for
// a URL with no cookies (#92).
func TestCookiesGetAndGetAll(t *testing.T) {
	cookies := func(rawURL string) []Cookie {
		if rawURL == "https://api/x" {
			return []Cookie{{Name: "session", Value: "xyz"}, {Name: "csrf", Value: "tok"}}
		}
		return nil
	}
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `
		helena.env.set("s", String(helena.cookies.get("https://api/x", "session")));
		helena.env.set("missing", String(helena.cookies.get("https://api/x", "nope")));
		var all = helena.cookies.getAll("https://api/x");
		helena.env.set("all", all.session + "," + all.csrf);
		helena.env.set("none", String(helena.cookies.get("https://other/", "session")));
		helena.env.set("emptyKeys", String(Object.keys(helena.cookies.getAll("https://other/")).length));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithCookies(cookies)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	want := map[string]string{"s": "xyz", "missing": "undefined", "all": "xyz,tok", "none": "undefined", "emptyKeys": "0"}
	for k, exp := range want {
		if v, _ := bridge.Get(k); v != exp {
			t.Errorf("%s = %q, want %q", k, v, exp)
		}
	}
}

// TestCookiesUnavailable verifies that with no lookup wired, get is undefined
// and getAll is an empty object (reading cookies is a no-op, not an error).
func TestCookiesUnavailable(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `
		helena.env.set("g", String(helena.cookies.get("https://x/", "a")));
		helena.env.set("n", String(Object.keys(helena.cookies.getAll("https://x/")).length));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("g"); v != "undefined" {
		t.Errorf("get without lookup = %q, want undefined", v)
	}
	if v, _ := bridge.Get("n"); v != "0" {
		t.Errorf("getAll without lookup keys = %q, want 0", v)
	}
}
