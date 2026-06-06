package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestSnapshotRequestDetachesSlices pins the Send-path race fix: the snapshot
// handed to the off-UI worker must not share any backing array with the live
// request, so concurrent UI-thread edits during an in-flight send can't race
// (or feed torn values into) the worker.
func TestSnapshotRequestDetachesSlices(t *testing.T) {
	orig := model.Request{
		Params:  []model.KeyValue{{Enabled: true, Key: "a", Value: "1"}},
		Headers: []model.KeyValue{{Enabled: true, Key: "h", Value: "v"}},
		Body:    model.Body{Form: []model.KeyValue{{Enabled: true, Key: "f", Value: "x"}}},
		Chain:   []model.ChainStep{{Alias: "login", Request: "Auth/Login"}},
	}
	snap := snapshotRequest(orig)

	// In-place edits to the original must not bleed into the snapshot.
	orig.Params[0].Value = "MUT"
	orig.Headers[0].Value = "MUT"
	orig.Body.Form[0].Value = "MUT"
	orig.Chain[0].Request = "MUT"
	if snap.Params[0].Value != "1" || snap.Headers[0].Value != "v" ||
		snap.Body.Form[0].Value != "x" || snap.Chain[0].Request != "Auth/Login" {
		t.Errorf("snapshot shares a backing array with the original: %+v", snap)
	}

	// Append-growth on the original (slices.Delete / append in the KV/Chain
	// editors) must not alias the snapshot either.
	orig.Chain = append(orig.Chain, model.ChainStep{Alias: "x"})
	orig.Params = append(orig.Params, model.KeyValue{Key: "b"})
	if len(snap.Chain) != 1 || len(snap.Params) != 1 {
		t.Errorf("snapshot grew with the original: chain=%d params=%d", len(snap.Chain), len(snap.Params))
	}
}
