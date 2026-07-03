package httpclient

// Temporary reproduction for the reviewer-claimed header-timer race.
// DELETE AFTER USE — not part of the change under review.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/sse"
	"github.com/idct/helena/internal/vars"
)

// TestZZHeaderTimerRaceRepro lands response headers at ~exactly the client
// timeout, many times. Legitimate outcomes per the new contract:
//   - "sse: no response within ..." (timer won the header phase), or
//   - a fully successful stream (headers won, timer stopped).
//
// The claimed bug: Do returns success but the already-fired timer cancels
// sctx, so the just-opened stream dies with "sse: ... context canceled"
// (onOpen already fired / status 200 accepted).
func TestZZHeaderTimerRaceRepro(t *testing.T) {
	const d = 30 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(d) // headers land at ~exactly the timeout
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: hello\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	c := New(model.Settings{TimeoutSeconds: 1})
	c.http.Timeout = d // millisecond-scale timeout; unexported, in-package only

	var headerTimeouts, successes, opened int64
	for i := 0; i < 400; i++ {
		var didOpen atomic.Bool
		err := c.Stream(context.Background(),
			model.Request{Method: model.GET, URL: srv.URL}, vars.New(),
			func(StreamMeta) { didOpen.Store(true) },
			func(sse.Event) bool { return true })
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "no response within"):
			if didOpen.Load() {
				t.Fatalf("iter %d: onOpen fired but got header-timeout error %v", i, err)
			}
			headerTimeouts++
		default:
			if didOpen.Load() {
				opened++
				t.Logf("iter %d: BUG REPRODUCED: stream opened (onOpen fired) then died: %v", i, err)
			} else {
				t.Logf("iter %d: other pre-open error: %v", i, err)
			}
		}
	}
	t.Logf("successes=%d headerTimeouts=%d opened-then-killed=%d", successes, headerTimeouts, opened)
	if opened > 0 {
		t.Fatalf("reproduced %d times: open stream killed by header timer", opened)
	}
}
