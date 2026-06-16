package examples_test

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/examples"
	"github.com/idct/helena/internal/storage"
)

// TestWriteSampleMaterializesAndLoads verifies the embedded sample collection
// can be written to disk and loaded through the real storage layer — the path
// a binary-only user takes via "Load sample" (#57).
func TestWriteSampleMaterializesAndLoads(t *testing.T) {
	dir, err := examples.WriteSample(t.TempDir())
	if err != nil {
		t.Fatalf("WriteSample: %v", err)
	}
	if filepath.Base(dir) != examples.SampleName {
		t.Errorf("sample dir = %q, want basename %q", dir, examples.SampleName)
	}

	c, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load materialized sample: %v", err)
	}
	if len(c.Requests) < 2 {
		t.Errorf("sample has %d requests, want >= 2", len(c.Requests))
	}
	if len(c.Environments) == 0 {
		t.Error("sample has no environments")
	}

	// Idempotent: a second write overwrites cleanly and still loads.
	if _, err := examples.WriteSample(filepath.Dir(dir)); err != nil {
		t.Fatalf("WriteSample (second): %v", err)
	}
}
