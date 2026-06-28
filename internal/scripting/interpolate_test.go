package scripting

import (
	"context"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestInterpolateUsesInjectedResolver verifies helena.interpolate(template) runs
// the argument through the WithInterpolator function (#92).
func TestInterpolateUsesInjectedResolver(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	interp := func(s string) string { return strings.ReplaceAll(s, "{{name}}", "Helena") }
	src := `helena.env.set("out", helena.interpolate("hi {{name}}!"));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithInterpolator(interp)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("out"); v != "hi Helena!" {
		t.Errorf("interpolate = %q, want %q", v, "hi Helena!")
	}
}

// TestInterpolateIdentityWithoutResolver verifies interpolate returns its input
// unchanged when no interpolator is injected (so the binding is always present).
func TestInterpolateIdentityWithoutResolver(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `helena.env.set("out", helena.interpolate("{{unchanged}}"));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("out"); v != "{{unchanged}}" {
		t.Errorf("interpolate without resolver = %q, want it unchanged", v)
	}
}

// TestInterpolateSeesInScriptEnvSet verifies interpolate resolves against live
// state: a helena.env.set earlier in the same script is visible because the
// injected resolver is consulted at call time (mirrors the per-call resolver
// rebuild the UI/runner wire in). The injected fn here reads the same bridge the
// script writes to.
func TestInterpolateSeesInScriptEnvSet(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	// Resolver that looks up {{NAME}} in the live bridge.
	interp := func(s string) string {
		key := strings.Trim(s, "{} ")
		v, _ := bridge.Get(key)
		return v
	}
	src := `helena.env.set("tok", "abc123"); helena.env.set("out", helena.interpolate("{{tok}}"));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithInterpolator(interp)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("out"); v != "abc123" {
		t.Errorf("interpolate did not see in-script env.set: got %q, want abc123", v)
	}
}

// TestInterpolateInPostResponse verifies interpolate is bound post-response too.
func TestInterpolateInPostResponse(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	in := ResponseInput{StatusCode: 200, Body: []byte("{}")}
	interp := func(s string) string { return "resolved:" + s }
	res, err := rt.RunPostResponse(context.Background(), `console.log(helena.interpolate("z"));`, r, in, nil, WithInterpolator(interp))
	if err != nil {
		t.Fatalf("RunPostResponse: %v", err)
	}
	if len(res.Console) != 1 || res.Console[0] != "resolved:z" {
		t.Errorf("console = %v, want [resolved:z]", res.Console)
	}
}
