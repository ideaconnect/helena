package ui

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestReclaimBelowThreshold: a small freed body must not trigger a GC (no goroutine
// is even launched, so the count stays 0 synchronously).
func TestReclaimBelowThreshold(t *testing.T) {
	orig := freeOSMemory
	defer func() { freeOSMemory = orig }()
	memTrimRunning.Store(false)

	var calls atomic.Int32
	freeOSMemory = func() { calls.Add(1) }

	reclaimAfterLargeBody(memTrimThreshold - 1)
	if got := calls.Load(); got != 0 {
		t.Fatalf("freed=%d triggered %d reclaims, want 0", memTrimThreshold-1, got)
	}
}

// TestReclaimAboveThreshold: a large freed body reclaims exactly once.
func TestReclaimAboveThreshold(t *testing.T) {
	orig := freeOSMemory
	defer func() { freeOSMemory = orig }()
	memTrimRunning.Store(false)

	var calls atomic.Int32
	done := make(chan struct{}, 1)
	freeOSMemory = func() { calls.Add(1); done <- struct{}{} }

	reclaimAfterLargeBody(memTrimThreshold)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reclaim did not run within 2s")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("reclaim calls=%d, want 1", got)
	}
}

// TestReclaimSingleFlight: while a reclaim is in flight, further requests are
// dropped rather than piling up back-to-back GCs.
func TestReclaimSingleFlight(t *testing.T) {
	orig := freeOSMemory
	defer func() { freeOSMemory = orig }()
	memTrimRunning.Store(false)

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	freeOSMemory = func() { calls.Add(1); started <- struct{}{}; <-release }

	reclaimAfterLargeBody(memTrimThreshold) // launches; blocks inside freeOSMemory
	<-started
	reclaimAfterLargeBody(memTrimThreshold) // dropped (in flight)
	reclaimAfterLargeBody(memTrimThreshold) // dropped
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for memTrimRunning.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("single-flight: calls=%d, want 1", got)
	}
}
