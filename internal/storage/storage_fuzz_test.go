package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzReadCollectionFile feeds arbitrary bytes to the collection
// file parser looking for crashes, panics, or out-of-memory growth.
// readCollectionFile must always return — either with a parsed DTO
// or with a non-nil error from yaml.Unmarshal.
//
// The parser surface is the load gateway: every byte sequence
// Helena ever sees as a collection file flows through here, and
// hostile inputs from a downloaded collection must NOT crash the
// process.
func FuzzReadCollectionFile(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(""),
		[]byte("info:\n  name: A\n"),
		[]byte("not yaml"),
		[]byte("info: [unterminated"),
		[]byte("info:\n  name:\n    nested: x"),
		[]byte("\x00\xff\xfe"),
		[]byte("info:\n  name: " + repeat("a", 1024) + "\n"),
		[]byte("---\n---\n---\n"),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "opencollection.yml")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Skipf("can't write fuzz input: %v", err)
		}
		_, _ = readCollectionFile(path)
		// No panic means pass. The error may or may not be nil
		// depending on YAML validity; both are acceptable.
	})
}

// FuzzReadRequestFile mirrors FuzzReadCollectionFile for the
// per-request file parser. Request files carry the most complex
// shape (info + http + scripts + chain + Extras) so this is the
// surface most likely to misbehave on adversarial input.
func FuzzReadRequestFile(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(""),
		[]byte("info:\n  name: R\nhttp:\n  method: GET\n  url: https://x/\n"),
		[]byte("http:\n  method: ?\\n  url: notaurl\n"),
		[]byte("scripts:\n  preRequest: |\n    throw 1\n"),
		[]byte("chain:\n  - alias: a\n    request: B\n"),
		[]byte("chain:\n  - {}\n"),
		[]byte("http: 42\n"),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "r.yml")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Skipf("can't write fuzz input: %v", err)
		}
		_, _ = readRequestFile(path)
	})
}

// FuzzLoadCollection drives the full Load path with a hand-authored
// directory: a collection.yml plus one request file. Exercises the
// glue between Load, loadItems, loadEnvironments, and the DTO
// converters at once.
func FuzzLoadCollection(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("info:\n  name: A\n  type: collection\n"),
		[]byte("info:\n  name: A\nfolders:\n  - missing\n"),
		[]byte(""),
		[]byte("info:\n  name: A\n  type: collection\nauth:\n  type: bearer\n  bearer:\n    token: t\n"),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, collBody []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), collBody, 0o644); err != nil {
			t.Skipf("write collection: %v", err)
		}
		_, _ = Load(dir)
	})
}

// repeat returns s concatenated n times. Used by FuzzReadCollectionFile
// seeds to feed the parser an unusually long literal.
func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
