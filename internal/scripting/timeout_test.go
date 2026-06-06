package scripting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// swapTimeout temporarily shortens ScriptTimeout + interruptGrace for fast
// tests and returns a restore func.
func swapTimeout(d time.Duration) func() {
	ot, og := ScriptTimeout, interruptGrace
	ScriptTimeout, interruptGrace = d, 100*time.Millisecond
	return func() { ScriptTimeout, interruptGrace = ot, og }
}

// TestRunWithTimeoutInterruptsInfiniteLoop covers the ordinary path: an
// interruptible JS loop is stopped by goja's Interrupt at a checkpoint.
func TestRunWithTimeoutInterruptsInfiniteLoop(t *testing.T) {
	defer swapTimeout(50 * time.Millisecond)()
	start := time.Now()
	err := runWithTimeout(context.Background(), goja.New(), "while(true){}")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v; interrupt should be prompt", d)
	}
}

// TestRunWithTimeoutAbandonsStuckNativeCall is the load-bearing case: a script
// blocked inside a native built-in ignores goja's Interrupt, so runWithTimeout
// must return after the grace period rather than freezing the caller forever.
func TestRunWithTimeoutAbandonsStuckNativeCall(t *testing.T) {
	defer swapTimeout(50 * time.Millisecond)()
	vm := goja.New()
	block := make(chan struct{})
	defer close(block) // releases the orphaned goroutine at test end
	if err := vm.Set("block", func() { <-block }); err != nil {
		t.Fatalf("bind: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- runWithTimeout(context.Background(), vm, "block()") }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("err = %v, want timeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runWithTimeout never returned — abandon path failed (caller frozen)")
	}
}

// TestRunWithTimeoutCancelAbandonsStuckNativeCall verifies the same abandon
// behaviour for context cancellation (with a long ScriptTimeout, so cancel
// wins) against a script stuck in native code.
func TestRunWithTimeoutCancelAbandonsStuckNativeCall(t *testing.T) {
	defer swapTimeout(5 * time.Second)()
	vm := goja.New()
	block := make(chan struct{})
	defer close(block)
	if err := vm.Set("block", func() { <-block }); err != nil {
		t.Fatalf("bind: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	done := make(chan error, 1)
	go func() { done <- runWithTimeout(ctx, vm, "block()") }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("err = %v, want cancelled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runWithTimeout never returned on cancel + stuck native call")
	}
}
