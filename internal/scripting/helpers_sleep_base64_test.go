package scripting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// TestHelperBase64RoundTrip verifies helena.base64.encode/decode (#92).
func TestHelperBase64RoundTrip(t *testing.T) {
	if got := runHelper(t, `helena.base64.encode("hello")`); got != "aGVsbG8=" {
		t.Errorf("base64.encode = %q, want aGVsbG8=", got)
	}
	if got := runHelper(t, `helena.base64.decode("aGVsbG8=")`); got != "hello" {
		t.Errorf("base64.decode = %q, want hello", got)
	}
	// Round-trip with bytes that need padding.
	if got := runHelper(t, `helena.base64.decode(helena.base64.encode("Helena!"))`); got != "Helena!" {
		t.Errorf("base64 round-trip = %q, want Helena!", got)
	}
}

// TestHelperBase64DecodeThrows verifies invalid input throws a catchable error.
func TestHelperBase64DecodeThrows(t *testing.T) {
	got := runHelper(t, `(function(){ try { helena.base64.decode("@@not-base64@@"); return "no-throw"; } catch (e) { return "threw"; } })()`)
	if got != "threw" {
		t.Errorf("base64.decode of garbage = %q, want threw", got)
	}
}

// TestHelperSleepDelays verifies helena.sleep blocks roughly the requested time.
func TestHelperSleepDelays(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	start := time.Now()
	if _, err := rt.RunPreRequest(context.Background(), `helena.sleep(60);`, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("sleep(60) returned after only %v, want >=40ms", elapsed)
	}
}

// TestHelperSleepNonPositiveNoop verifies a non-positive argument returns at once.
func TestHelperSleepNonPositiveNoop(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	start := time.Now()
	if _, err := rt.RunPreRequest(context.Background(), `helena.sleep(-5); helena.sleep(0);`, &r, nil); err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
		t.Errorf("non-positive sleep took %v, want near-zero", elapsed)
	}
}

// TestHelperSleepAbortsOnCancel verifies a cancelled context unblocks sleep
// promptly rather than running the full requested duration.
func TestHelperSleepAbortsOnCancel(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	start := time.Now()
	// 4s is within ScriptTimeout (5s) so the cap doesn't apply; cancel must end it.
	_, _ = rt.RunPreRequest(ctx, `helena.sleep(4000);`, &r, nil)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("sleep did not abort on cancel: ran %v", elapsed)
	}
}

// TestHelperSleepBoundInPostResponse verifies sleep is also available post-response.
func TestHelperSleepBoundInPostResponse(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	in := ResponseInput{StatusCode: 200, Body: []byte("{}")}
	res, err := rt.RunPostResponse(context.Background(), `helena.sleep(1); console.log("ok");`, r, in, nil)
	if err != nil {
		t.Fatalf("RunPostResponse: %v", err)
	}
	if len(res.Console) != 1 || !strings.Contains(res.Console[0], "ok") {
		t.Errorf("console = %v, want [ok]", res.Console)
	}
}
