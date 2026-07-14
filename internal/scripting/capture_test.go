package scripting

import (
	"context"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestConsoleCaptureCappedByLines pins the line cap: a runaway console.log loop
// must not grow res.Console without bound — it stops at the cap plus one
// truncation marker.
func TestConsoleCaptureCappedByLines(t *testing.T) {
	defer swapCaps(10, 1<<20, 1000)()
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "x"}
	res, err := rt.RunPreRequest(context.Background(), `for (var i=0;i<100000;i++) console.log("line"+i);`, &r, nil)
	if err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if len(res.Console) != maxConsoleLines+1 {
		t.Fatalf("console lines = %d, want %d (cap + marker)", len(res.Console), maxConsoleLines+1)
	}
	if got := res.Console[len(res.Console)-1]; got != consoleTruncatedMsg {
		t.Errorf("last line = %q, want the truncation marker", got)
	}
}

// TestConsoleCaptureCappedByBytes pins the byte cap: a few huge lines trip the
// total-bytes limit even under the line cap.
func TestConsoleCaptureCappedByBytes(t *testing.T) {
	defer swapCaps(100000, 100, 1000)()
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "x"}
	res, err := rt.RunPreRequest(context.Background(), `for (var i=0;i<1000;i++) console.log("0123456789");`, &r, nil)
	if err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if got := res.Console[len(res.Console)-1]; got != consoleTruncatedMsg {
		t.Errorf("last line = %q, want the truncation marker", got)
	}
	total := 0
	for _, l := range res.Console {
		if l != consoleTruncatedMsg {
			total += len(l)
		}
	}
	if total > maxConsoleBytes {
		t.Errorf("captured %d bytes, want <= %d", total, maxConsoleBytes)
	}
}

// TestTestResultsCapped pins the test-result cap: a runaway test() loop stops
// growing res.Tests at the cap.
func TestTestResultsCapped(t *testing.T) {
	defer swapCaps(1000, 1<<20, 10)()
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "x"}
	res, err := rt.RunPostResponse(context.Background(), `for (var i=0;i<1000;i++) test("t"+i, function(){});`, r, ResponseInput{}, nil)
	if err != nil {
		t.Fatalf("RunPostResponse: %v", err)
	}
	if len(res.Tests) != maxTestResults {
		t.Errorf("test results = %d, want capped at %d", len(res.Tests), maxTestResults)
	}
}

// swapCaps temporarily shrinks the capture caps and returns a restore func.
func swapCaps(lines, bytes, tests int) func() {
	ol, ob, ot := maxConsoleLines, maxConsoleBytes, maxTestResults
	maxConsoleLines, maxConsoleBytes, maxTestResults = lines, bytes, tests
	return func() { maxConsoleLines, maxConsoleBytes, maxTestResults = ol, ob, ot }
}
