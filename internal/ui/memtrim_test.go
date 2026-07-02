package ui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
)

// captureReclaim swaps the freeOSMemory seam for a channel-backed spy and
// returns the channel plus a wait helper; the caller's cleanup restores it.
func captureReclaim(t *testing.T) (calls *atomic.Int32, wait func() bool) {
	t.Helper()
	orig := freeOSMemory
	memTrimRunning.Store(false)
	var n atomic.Int32
	fired := make(chan struct{}, 8)
	freeOSMemory = func() { n.Add(1); fired <- struct{}{} }
	t.Cleanup(func() { freeOSMemory = orig })
	return &n, func() bool {
		select {
		case <-fired:
			return true
		case <-time.After(2 * time.Second):
			return false
		}
	}
}

// bigBody is a cached-response body exactly at the reclaim threshold.
func bigBody() string { return strings.Repeat("x", memTrimThreshold) }

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

// TestDeliverResponseInactiveTabReclaims: replacing a large cached response on a
// tab that is NOT displayed must reclaim (applyResponse never runs there).
func TestDeliverResponseInactiveTabReclaims(t *testing.T) {
	m, _, _ := newTabUI(t)
	calls, wait := captureReclaim(t)

	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 (active)
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: bigBody(), status: "200"})
	if got := calls.Load(); got != 0 {
		t.Fatalf("first (nothing replaced) deliver fired %d reclaims, want 0", got)
	}
	// Replacing the big cached body on the inactive tab must trigger.
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: "tiny", status: "200"})
	if !wait() {
		t.Fatal("replacing a large cached body on an inactive tab did not reclaim")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("reclaims = %d, want 1", got)
	}
}

// TestCloseTabReclaimsCachedBody: closing a non-active tab drops its large
// cached response and must reclaim.
func TestCloseTabReclaimsCachedBody(t *testing.T) {
	m, _, _ := newTabUI(t)
	calls, wait := captureReclaim(t)

	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 (active)
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: bigBody(), status: "200"})

	m.closeTab(m.tabs[0])
	if !wait() {
		t.Fatal("closing a tab with a large cached body did not reclaim")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("reclaims = %d, want 1", got)
	}
}

// TestCloseAllTabsSumsCachedBodies: two cached bodies each below the threshold
// must reclaim when their sum crosses it.
func TestCloseAllTabsSumsCachedBodies(t *testing.T) {
	m, _, _ := newTabUI(t)
	calls, wait := captureReclaim(t)

	half := strings.Repeat("y", memTrimThreshold/2)
	m.openOrActivate("0/r0")
	m.openOrActivate("0/r1")
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: half, status: "200"})
	m.deliverResponse(m.tabs[1], &tabResponse{rawBody: half, status: "200"})
	if got := calls.Load(); got != 0 {
		t.Fatalf("setup fired %d reclaims, want 0 (each body is below threshold)", got)
	}

	m.closeAllTabs()
	if !wait() {
		t.Fatal("closeAllTabs did not reclaim the summed cached bodies")
	}
}

// TestReconcileTabsReclaimsDroppedTabs: a tab dropped because its request no
// longer resolves discards its large cached response and must reclaim.
func TestReconcileTabsReclaimsDroppedTabs(t *testing.T) {
	m, _, _ := newTabUI(t)
	calls, wait := captureReclaim(t)

	m.openOrActivate("0/r0") // tab 0
	m.openOrActivate("0/r1") // tab 1 (active)
	m.deliverResponse(m.tabs[0], &tabResponse{rawBody: bigBody(), status: "200"})
	m.tabs[0].collection = "/nonexistent" // request can no longer be located

	m.reconcileTabs()
	if !wait() {
		t.Fatal("reconcileTabs dropping a tab with a large cached body did not reclaim")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("reclaims = %d, want 1", got)
	}
}

// TestStreamStartReclaimsPreviousBody: starting a stream blanks the shared
// response viewer; a large previous body must be reclaimed. The per-event
// SetData inside the stream loop must NOT reclaim (hot path).
func TestStreamStartReclaimsPreviousBody(t *testing.T) {
	m := newAuthUI(t)
	calls, wait := captureReclaim(t)

	m.pv.SetData([]byte(bigBody()), prettyview.FormatRaw)
	m.URL.SetText("http://127.0.0.1:9") // discard port: the worker fails fast
	m.streamSend()
	if !wait() {
		t.Fatal("stream start did not reclaim the previous large body")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("reclaims = %d, want 1", got)
	}
	if m.streamCancel != nil {
		m.streamCancel()
	}
}
