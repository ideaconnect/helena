package scripting

import (
	"context"
	"testing"

	"github.com/idct/helena/internal/model"
)

type fakeRunner struct{ stops, skips int }

func (f *fakeRunner) Stop() { f.stops++ }
func (f *fakeRunner) Skip() { f.skips++ }

// TestRunnerControlInjected verifies helena.runner.stop()/skip() invoke the
// injected RunnerControl (#92).
func TestRunnerControlInjected(t *testing.T) {
	fr := &fakeRunner{}
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `helena.runner.stop(); helena.runner.skip(); helena.runner.skip();`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil, WithRunner(fr)); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if fr.stops != 1 || fr.skips != 2 {
		t.Errorf("stops=%d skips=%d, want 1 and 2", fr.stops, fr.skips)
	}
}

// TestRunnerControlNoopWhenUnwired verifies helena.runner.* are harmless no-ops
// when no runner is wired (the UI Send case) — they must not throw.
func TestRunnerControlNoopWhenUnwired(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `helena.runner.stop(); helena.runner.skip(); helena.env.set("ok", "1");`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if v, _ := bridge.Get("ok"); v != "1" {
		t.Errorf("script after no-op runner calls did not continue: ok=%q", v)
	}
}
