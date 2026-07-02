package ui

import (
	"runtime/debug"
	"sync/atomic"
)

// memTrimThreshold is the freed-body size above which we bother returning memory
// to the OS. debug.FreeOSMemory forces a full GC, so it isn't worth the pause for
// small responses; the win is reclaiming the (several-x-body-size) pretty-view
// buffers a large response leaves behind once it's replaced.
const memTrimThreshold = 8 << 20 // 8 MiB

// freeOSMemory is a seam so tests can observe the reclaim without paying a GC.
var freeOSMemory = debug.FreeOSMemory

var memTrimRunning atomic.Bool

// reclaimAfterLargeBody returns freed Go heap to the OS, off the UI thread, when a
// large response body has just been replaced or cleared. Go's scavenger is lazy,
// so without this the parsed-model buffers for a multi-MB body linger in the
// process RSS for many seconds after they become garbage — memory appears to
// ratchet up across a session of big responses. No-op below the threshold, and
// single-flighted so rapid sends don't pile up back-to-back GCs.
func reclaimAfterLargeBody(freedBytes int) {
	if freedBytes < memTrimThreshold {
		return
	}
	if !memTrimRunning.CompareAndSwap(false, true) {
		return // a reclaim is already in flight
	}
	go func() {
		defer memTrimRunning.Store(false)
		freeOSMemory()
	}()
}
