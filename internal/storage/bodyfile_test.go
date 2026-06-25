package storage

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestFileBodyRoundTrip pins #24: a BodyFile request's FilePath + ContentType
// survive Save -> Load.
func TestFileBodyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name:   "Upload",
			Method: model.POST,
			URL:    "https://x/upload",
			Body:   model.Body{Type: model.BodyFile, FilePath: "/tmp/payload.bin", ContentType: "image/png"},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := got.Requests[0].Body
	if b.Type != model.BodyFile || b.FilePath != "/tmp/payload.bin" || b.ContentType != "image/png" {
		t.Errorf("file body did not round-trip: %+v", b)
	}
}
