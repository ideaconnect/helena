// Package history keeps a bounded, restart-persistent log of sent requests and
// their response summaries (#65), so the user can revisit, restore, or resend a
// past request. It is an in-memory ring mirrored to a YAML file under the config
// dir; a Store is safe for concurrent use.
//
// Snapshots are secret-scrubbed (storage.ScrubRequestSecrets) before they are
// recorded, so history.yml never carries a cleartext credential — the same
// privacy invariant the collection YAML has (#42). Restore therefore keeps
// everything but the secret, which a resend re-resolves from the environment.
package history

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// DefaultMax is the entry cap used when New is given max <= 0.
const DefaultMax = 200

// Entry is one recorded send: the (secret-scrubbed) request as sent, enough to
// restore or resend it, plus a summary of the response.
type Entry struct {
	Time     time.Time     `yaml:"time"`
	Method   string        `yaml:"method"`
	URL      string        `yaml:"url"`
	Status   int           `yaml:"status,omitempty"`   // 0 when the send errored
	Duration time.Duration `yaml:"duration,omitempty"` // round-trip time
	Size     int64         `yaml:"size,omitempty"`     // response body bytes
	Err      string        `yaml:"err,omitempty"`      // non-empty on a failed send
	Request  model.Request `yaml:"request"`            // scrubbed of secrets
}

// Store is a bounded, restart-persistent history of sends. The in-memory slice
// is oldest-first; Entries returns it newest-first for display.
type Store struct {
	mu      sync.Mutex
	path    string // "" disables persistence (in-memory only)
	max     int
	entries []Entry
}

type fileFormat struct {
	Entries []Entry `yaml:"entries"`
}

// New returns a Store backed by path (a YAML file), loading any existing
// history. A blank path keeps history in memory only. max <= 0 uses DefaultMax.
func New(path string, max int) *Store {
	if max <= 0 {
		max = DefaultMax
	}
	s := &Store{path: path, max: max}
	s.load()
	return s
}

// Record appends a send to the history: the request is secret-scrubbed, a zero
// Time is stamped now, the ring is trimmed to the cap (oldest dropped), and the
// file is rewritten. Persistence is best-effort — a write failure is swallowed
// since history is a convenience, not load-bearing state.
func (s *Store) Record(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	e.Request = storage.ScrubRequestSecrets(e.Request)
	s.entries = append(s.entries, e)
	if len(s.entries) > s.max {
		s.entries = append(s.entries[:0], s.entries[len(s.entries)-s.max:]...)
	}
	s.save()
}

// Entries returns a newest-first deep copy of the recorded entries. Each
// returned Entry.Request is Cloned so a consumer that binds it into an editor
// (Restore / Resend) and edits in place cannot mutate the stored entry — an
// in-place header/param edit would otherwise rewrite history to a value that was
// never sent and persist the corruption on the next save.
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		e.Request = e.Request.Clone()
		out[len(s.entries)-1-i] = e
	}
	return out
}

// Len reports how many entries are recorded.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Clear empties the history and rewrites the file.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.save()
}

// load reads the YAML file into entries, best-effort: a missing, unreadable, or
// malformed file leaves the history empty. Over-cap files are trimmed to the
// newest max on load (in case the cap shrank between runs).
func (s *Store) load() {
	if s.path == "" {
		return
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var f fileFormat
	if yaml.Unmarshal(b, &f) != nil {
		return
	}
	s.entries = f.Entries
	if len(s.entries) > s.max {
		s.entries = s.entries[len(s.entries)-s.max:]
	}
}

// save atomically rewrites the file (stage + rename) so a crash mid-write can't
// truncate it. The caller holds the lock. Best-effort; errors are dropped.
func (s *Store) save() {
	if s.path == "" {
		return
	}
	b, err := yaml.Marshal(fileFormat{Entries: s.entries})
	if err != nil {
		return
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(b)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
	}
}
