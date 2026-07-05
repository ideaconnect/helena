package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

func req(method, url string) model.Request {
	return model.Request{Method: model.Method(method), URL: url}
}

// TestRecordAndEntriesNewestFirst: records accumulate and Entries returns them
// newest-first.
func TestRecordAndEntriesNewestFirst(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "history.yml"), 0)
	s.Record(Entry{Method: "GET", URL: "https://x/1", Request: req("GET", "https://x/1")})
	s.Record(Entry{Method: "POST", URL: "https://x/2", Request: req("POST", "https://x/2")})

	got := s.Entries()
	if len(got) != 2 {
		t.Fatalf("Entries len = %d, want 2", len(got))
	}
	if got[0].URL != "https://x/2" || got[1].URL != "https://x/1" {
		t.Errorf("order = [%s, %s], want newest-first [x/2, x/1]", got[0].URL, got[1].URL)
	}
	if got[0].Time.IsZero() {
		t.Errorf("zero Time was not stamped on record")
	}
}

// TestEntriesDeepCopyIsolatesStore pins that a caller editing a returned
// Entry.Request in place (as Restore/Resend do when they bind it into the
// editor) cannot mutate the stored history — a header edit on the returned copy
// must not rewrite what the next Entries() call reports.
func TestEntriesDeepCopyIsolatesStore(t *testing.T) {
	s := New("", 0)
	r := model.Request{Method: "GET", URL: "https://x/1",
		Headers: []model.KeyValue{{Key: "X", Value: "orig"}}}
	s.Record(Entry{Method: "GET", URL: r.URL, Request: r})

	got := s.Entries()
	got[0].Request.Headers[0].Value = "MUT" // simulate a post-Restore in-place edit

	again := s.Entries()
	if again[0].Request.Headers[0].Value != "orig" {
		t.Errorf("editing a returned entry corrupted the store: got %q, want %q",
			again[0].Request.Headers[0].Value, "orig")
	}
}

// TestBoundedDropsOldest: the ring keeps only the newest `max` entries.
func TestBoundedDropsOldest(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "history.yml"), 3)
	for i := 0; i < 5; i++ {
		u := "https://x/" + string(rune('0'+i))
		s.Record(Entry{Method: "GET", URL: u, Request: req("GET", u)})
	}
	got := s.Entries()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (capped)", len(got))
	}
	// Newest-first: x/4, x/3, x/2 survive; x/0, x/1 dropped.
	if got[0].URL != "https://x/4" || got[2].URL != "https://x/2" {
		t.Errorf("survivors = [%s .. %s], want [x/4 .. x/2]", got[0].URL, got[2].URL)
	}
}

// TestPersistReload: a fresh Store over the same path sees the recorded history.
func TestPersistReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.yml")
	s := New(path, 0)
	s.Record(Entry{Method: "GET", URL: "https://x/keep", Status: 200,
		Duration: 5 * time.Millisecond, Size: 12, Request: req("GET", "https://x/keep")})

	reloaded := New(path, 0)
	got := reloaded.Entries()
	if len(got) != 1 {
		t.Fatalf("reloaded len = %d, want 1", len(got))
	}
	if got[0].URL != "https://x/keep" || got[0].Status != 200 || got[0].Size != 12 {
		t.Errorf("reloaded entry = %+v, want the persisted GET x/keep 200 size 12", got[0])
	}
}

// TestClear empties the store and the file.
func TestClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.yml")
	s := New(path, 0)
	s.Record(Entry{Method: "GET", URL: "https://x/1", Request: req("GET", "https://x/1")})
	s.Clear()
	if s.Len() != 0 {
		t.Fatalf("after Clear Len = %d, want 0", s.Len())
	}
	if n := New(path, 0).Len(); n != 0 {
		t.Errorf("reloaded after Clear Len = %d, want 0", n)
	}
}

// TestRecordScrubsSecrets: a recorded snapshot must not carry credentials —
// auth secrets and Secret-flagged request vars are blanked (via
// storage.ScrubRequestSecrets), non-secret fields preserved.
func TestRecordScrubsSecrets(t *testing.T) {
	r := req("GET", "https://x/secure")
	r.Auth = model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "s3cr3t-token"}}
	r.Variables = []model.Variable{
		{Key: "pub", Value: "visible"},
		{Key: "tok", Value: "hidden", Secret: true},
	}
	s := New(filepath.Join(t.TempDir(), "history.yml"), 0)
	s.Record(Entry{Method: "GET", URL: r.URL, Request: r})

	got := s.Entries()[0].Request
	if got.Auth.Bearer == nil || got.Auth.Bearer.Token != "" {
		t.Errorf("bearer token not scrubbed: %+v", got.Auth.Bearer)
	}
	if got.Auth.Type != model.AuthBearer {
		t.Errorf("auth type lost: %v", got.Auth.Type)
	}
	for _, v := range got.Variables {
		if v.Secret && v.Value != "" {
			t.Errorf("secret var %q not scrubbed: %q", v.Key, v.Value)
		}
		if !v.Secret && v.Value == "" {
			t.Errorf("non-secret var %q was blanked", v.Key)
		}
	}
	// The caller's original request must be untouched (scrub works on a copy).
	if r.Auth.Bearer.Token != "s3cr3t-token" {
		t.Errorf("scrub mutated the caller's request: token = %q", r.Auth.Bearer.Token)
	}
}

// TestInMemoryNoPath: a blank path keeps history in memory without touching
// disk, and reload yields nothing.
func TestInMemoryNoPath(t *testing.T) {
	s := New("", 0)
	s.Record(Entry{Method: "GET", URL: "https://x/1", Request: req("GET", "https://x/1")})
	if s.Len() != 1 {
		t.Fatalf("in-memory Len = %d, want 1", s.Len())
	}
	if n := New("", 0).Len(); n != 0 {
		t.Errorf("a second in-memory store sees %d entries, want 0", n)
	}
}

// TestLoadMalformedFileIsEmpty: a garbage file yields an empty history, not a
// crash.
func TestLoadMalformedFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.yml")
	if err := os.WriteFile(path, []byte("}{not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := New(path, 0).Len(); n != 0 {
		t.Errorf("malformed file gave %d entries, want 0", n)
	}
}

// TestLoadTrimsOverCap: a file holding more than the (possibly shrunk) cap is
// trimmed to the newest max on load.
func TestLoadTrimsOverCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.yml")
	s := New(path, 0) // default cap: record 5
	for i := 0; i < 5; i++ {
		u := "https://x/" + string(rune('0'+i))
		s.Record(Entry{Method: "GET", URL: u, Request: req("GET", u)})
	}
	capped := New(path, 3) // reopen with a smaller cap → load trims to newest 3
	got := capped.Entries()
	if len(got) != 3 {
		t.Fatalf("loaded len = %d, want 3 (trimmed on load)", len(got))
	}
	if got[0].URL != "https://x/4" {
		t.Errorf("newest after trim = %s, want x/4", got[0].URL)
	}
}

// TestSaveFailureIsSwallowed: a persistence error (here the parent path is a
// regular file, so MkdirAll fails) must not panic or lose the in-memory entry.
func TestSaveFailureIsSwallowed(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The store's dir would be "blocker/", but blocker is a file → MkdirAll errs.
	s := New(filepath.Join(blocker, "history.yml"), 0)
	s.Record(Entry{Method: "GET", URL: "https://x/1", Request: req("GET", "https://x/1")})
	if s.Len() != 1 {
		t.Errorf("in-memory entry lost after a save failure: Len = %d", s.Len())
	}
}
