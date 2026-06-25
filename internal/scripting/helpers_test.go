package scripting

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// runHelper executes a one-line pre-request script that stashes a value via
// helena.env.set("out", ...) and returns what the bridge recorded for "out".
func runHelper(t *testing.T, expr string) string {
	t.Helper()
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	src := `helena.env.set("out", String(` + expr + `));`
	if _, err := rt.RunPreRequest(context.Background(), src, &r, nil); err != nil {
		t.Fatalf("RunPreRequest(%s): %v", expr, err)
	}
	v, ok := bridge.Get("out")
	if !ok {
		t.Fatalf("expr %s did not set out", expr)
	}
	return v
}

// TestHelperHashDigests verifies the md5/sha1/sha256/sha512 helpers produce the
// known hex digests for "abc".
func TestHelperHashDigests(t *testing.T) {
	cases := map[string]string{
		`helena.hash.md5("abc")`:    "900150983cd24fb0d6963f7d28e17f72",
		`helena.hash.sha1("abc")`:   "a9993e364706816aba3e25717850c26c9cd0d89d",
		`helena.hash.sha256("abc")`: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		`helena.hash.sha512("abc")`: "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
	}
	for expr, want := range cases {
		if got := runHelper(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

// TestHelperHMAC verifies hmacSha256/hmacSha1 against known RFC-style vectors
// (key "key", message "The quick brown fox jumps over the lazy dog").
func TestHelperHMAC(t *testing.T) {
	msg := "The quick brown fox jumps over the lazy dog"
	cases := map[string]string{
		`helena.hash.hmacSha256("key", "` + msg + `")`: "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		`helena.hash.hmacSha1("key", "` + msg + `")`:   "de7c9b85b8b78aa6bc8a7a36f70a90701c9db4d9",
	}
	for expr, want := range cases {
		if got := runHelper(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

// TestHelperUUID verifies helena.uuid() yields a well-formed v4 UUID and two
// calls differ.
func TestHelperUUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a := runHelper(t, `helena.uuid()`)
	b := runHelper(t, `helena.uuid()`)
	if !re.MatchString(a) {
		t.Errorf("uuid %q not a valid v4 UUID", a)
	}
	if a == b {
		t.Errorf("two uuids identical (%q) — not random", a)
	}
}

// TestHelperDate verifies helena.date.now() is parseable RFC 3339 and
// helena.date.timestamp() is a plausible recent Unix time.
func TestHelperDate(t *testing.T) {
	iso := runHelper(t, `helena.date.now()`)
	if _, err := time.Parse(time.RFC3339, iso); err != nil {
		t.Errorf("date.now() = %q, not RFC3339: %v", iso, err)
	}
	tsStr := runHelper(t, `helena.date.timestamp()`)
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		t.Fatalf("timestamp() = %q, not an integer: %v", tsStr, err)
	}
	if ts < 1_700_000_000 {
		t.Errorf("timestamp() = %d, implausibly old", ts)
	}
}

// TestHelpersBoundInPostResponse verifies the helper surface is available in
// the post-response phase too, not just pre-request.
func TestHelpersBoundInPostResponse(t *testing.T) {
	bridge := newFakeBridge()
	rt := New(bridge)
	r := model.Request{Method: model.GET, URL: "https://x/"}
	resp := ResponseInput{StatusCode: 200, Status: "200 OK", Body: []byte("{}"), Headers: http.Header{}}
	src := `helena.env.set("h", helena.hash.sha256("abc"));`
	if _, err := rt.RunPostResponse(context.Background(), src, r, resp, nil); err != nil {
		t.Fatalf("RunPostResponse: %v", err)
	}
	if got, _ := bridge.Get("h"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("post-response hash = %q", got)
	}
}
