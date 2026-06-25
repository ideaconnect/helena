package storage

import (
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAssertionsRoundTrip pins #88: a request's declarative assertions survive
// Save -> Load, including the enabled flag (stored inverted as `disabled`).
func TestAssertionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	asserts := []model.Assertion{
		{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"},
		{Enabled: false, Source: "res.json.token", Op: "exists"},
		{Enabled: true, Source: "res.body", Op: "contains", Expected: "ok"},
	}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R", Method: model.GET, URL: "https://x/", Assertions: asserts}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Requests[0].Assertions, asserts) {
		t.Errorf("assertions did not round-trip:\n got  %+v\n want %+v", got.Requests[0].Assertions, asserts)
	}
}
