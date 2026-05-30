package features

import (
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
)

// observedRequest records what arrived at one path on the test
// server: the headers, the parsed query values, and (for assertions
// that care) the path itself. Multiple hits to the same path append.
type observedRequest struct {
	Path    string
	Headers http.Header
	Query   url.Values
}

// handlerMux is a tiny http.Handler the feature suite uses as the
// test server's backend. It owns:
//
//   - A default handler that returns 200 "hello world".
//   - A per-path handler map registered via setHandler so scenarios
//     can describe specific responses with a Given step.
//   - A request count for assertions like "the test server
//     received no requests".
//   - A per-path observation log so scenarios can assert what the
//     chain step (or leaf) actually sent.
//
// Concurrent calls (Send goroutines, chain pre-fetches) are safe.
type handlerMux struct {
	mu       sync.RWMutex
	routes   map[string]http.HandlerFunc
	observed map[string][]observedRequest
	hits     atomic.Int64
}

func newHandlerMux() *handlerMux {
	return &handlerMux{
		routes:   map[string]http.HandlerFunc{},
		observed: map[string][]observedRequest{},
	}
}

// setHandler registers fn at path. Scenarios call this during their
// Given steps; subsequent ServeHTTP calls matching the path invoke fn
// instead of the default response.
func (m *handlerMux) setHandler(path string, fn http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[path] = fn
}

func (m *handlerMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.hits.Add(1)

	// Snapshot the headers + query before invoking the handler.
	hdr := r.Header.Clone()
	q := r.URL.Query()

	m.mu.Lock()
	m.observed[r.URL.Path] = append(m.observed[r.URL.Path], observedRequest{
		Path: r.URL.Path, Headers: hdr, Query: q,
	})
	fn, ok := m.routes[r.URL.Path]
	m.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
		return
	}
	fn(w, r)
}

// hitCount returns the total number of requests served since the mux
// was created. Used by the "the test server received no requests"
// step to verify pre-script failures short-circuit cleanly.
func (m *handlerMux) hitCount() int64 { return m.hits.Load() }

// observations returns the recorded requests for path (most recent
// last). Returns nil when the path was never hit.
func (m *handlerMux) observations(path string) []observedRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if got, ok := m.observed[path]; ok {
		// Return a copy so the caller can iterate without holding the lock.
		out := make([]observedRequest, len(got))
		copy(out, got)
		return out
	}
	return nil
}
