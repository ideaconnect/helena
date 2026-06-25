package assertion

import (
	"net/http"
	"testing"

	"github.com/idct/helena/internal/model"
)

func a(source, op, expected string) model.Assertion {
	return model.Assertion{Enabled: true, Source: source, Op: op, Expected: expected}
}

// TestEvaluateOperators exercises every operator against a representative
// response, asserting the expected pass/fail per row.
func TestEvaluateOperators(t *testing.T) {
	headers := http.Header{"Content-Type": {"application/json"}}
	body := []byte(`{"ok":true,"count":3,"name":"helena","items":[{"id":7}]}`)
	cases := []struct {
		name string
		a    model.Assertion
		want bool
	}{
		{"status equals", a("res.status", OpEquals, "200"), true},
		{"status notEquals", a("res.status", OpNotEquals, "500"), true},
		{"status greaterThan", a("res.status", OpGreater, "199"), true},
		{"status lessThan fails", a("res.status", OpLess, "100"), false},
		{"header equals", a("res.header.Content-Type", OpEquals, "application/json"), true},
		{"header contains", a("res.header.Content-Type", OpContains, "json"), true},
		{"body contains", a("res.body", OpContains, "helena"), true},
		{"body notContains", a("res.body", OpNotContains, "nope"), true},
		{"body matches", a("res.body", OpMatches, `"count":\s*3`), true},
		{"json equals", a("res.json.name", OpEquals, "helena"), true},
		{"json bool", a("res.json.ok", OpEquals, "true"), true},
		{"json number gt", a("res.json.count", OpGreater, "2"), true},
		{"json array index", a("res.json.items.0.id", OpEquals, "7"), true},
		{"json exists", a("res.json.name", OpExists, ""), true},
		{"json notExists", a("res.json.missing", OpNotExists, ""), true},
		{"json missing exists fails", a("res.json.missing", OpExists, ""), false},
		{"missing field comparison fails", a("res.json.missing", OpEquals, "x"), false},
		{"bad regex fails", a("res.body", OpMatches, "("), false},
		{"non-numeric gt fails", a("res.body", OpGreater, "5"), false},
	}
	for _, c := range cases {
		got := Evaluate([]model.Assertion{c.a}, 200, headers, body)
		if len(got) != 1 {
			t.Fatalf("%s: got %d results", c.name, len(got))
		}
		if got[0].Passed != c.want {
			t.Errorf("%s: passed=%v want %v (err=%q)", c.name, got[0].Passed, c.want, got[0].Error)
		}
		if !got[0].Passed && got[0].Error == "" {
			t.Errorf("%s: failing result missing an error message", c.name)
		}
	}
}

// TestEvaluateSkipsDisabled verifies disabled rows are not evaluated.
func TestEvaluateSkipsDisabled(t *testing.T) {
	got := Evaluate([]model.Assertion{
		{Enabled: false, Source: "res.status", Op: OpEquals, Expected: "999"},
		{Enabled: true, Source: "res.status", Op: OpEquals, Expected: "200"},
	}, 200, nil, nil)
	if len(got) != 1 || !got[0].Passed {
		t.Errorf("got %+v, want one passing result (disabled row skipped)", got)
	}
}

// TestJSONPathEdgeCases covers nested-object stringification, a null value,
// out-of-range / non-container path segments, and a non-JSON body.
func TestJSONPathEdgeCases(t *testing.T) {
	body := []byte(`{"obj":{"a":1},"n":null,"arr":[1]}`)
	cases := []struct {
		name string
		a    model.Assertion
		want bool
	}{
		{"object equals json", a("res.json.obj", OpEquals, `{"a":1}`), true},
		{"null exists", a("res.json.n", OpExists, ""), true},
		{"null equals", a("res.json.n", OpEquals, "null"), true},
		{"array out of range notExists", a("res.json.arr.5", OpNotExists, ""), true},
		{"descend into scalar notExists", a("res.json.n.x", OpNotExists, ""), true},
		{"array non-numeric index notExists", a("res.json.arr.k", OpNotExists, ""), true},
	}
	for _, c := range cases {
		got := Evaluate([]model.Assertion{c.a}, 200, nil, body)
		if got[0].Passed != c.want {
			t.Errorf("%s: passed=%v want %v (err=%q)", c.name, got[0].Passed, c.want, got[0].Error)
		}
	}
	// Non-JSON body: any res.json path is "not found".
	if got := Evaluate([]model.Assertion{a("res.json.x", OpExists, "")}, 200, nil, []byte("not json")); got[0].Passed {
		t.Errorf("res.json on non-JSON body should not exist: %+v", got)
	}
}

// TestEvaluateUnknownSourceAndOp verifies an unknown source is "not found" and
// an unknown operator fails cleanly.
func TestEvaluateUnknownSourceAndOp(t *testing.T) {
	got := Evaluate([]model.Assertion{
		{Enabled: true, Source: "res.bogus", Op: OpEquals, Expected: "x"},
		{Enabled: true, Source: "res.status", Op: "weird", Expected: "x"},
	}, 200, nil, nil)
	if got[0].Passed || got[1].Passed {
		t.Errorf("expected both to fail: %+v", got)
	}
}
