package scripting

import (
	"context"
	"net/http"
	"testing"

	"github.com/idct/helena/internal/model"
)

// runTests evaluates a post-response script and returns the recorded Tests.
func runTests(t *testing.T, src string) []TestResult {
	t.Helper()
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	resp := ResponseInput{StatusCode: 200, Status: "200 OK", Headers: http.Header{}, Body: []byte(`{"ok":true}`)}
	res, err := rt.RunPostResponse(context.Background(), src, r, resp, nil)
	if err != nil {
		t.Fatalf("RunPostResponse: %v", err)
	}
	return res.Tests
}

// TestAssertPassAndFail verifies test() records a pass for a satisfied
// assertion and a fail (with the matcher's message) for a violated one.
func TestAssertPassAndFail(t *testing.T) {
	got := runTests(t, `
		test("status is 200", function () { expect(response.status).toBe(200); });
		test("status is 201", function () { expect(response.status).toBe(201); });
	`)
	if len(got) != 2 {
		t.Fatalf("recorded %d tests, want 2: %+v", len(got), got)
	}
	if !got[0].Passed || got[0].Name != "status is 200" {
		t.Errorf("test[0] = %+v, want passed 'status is 200'", got[0])
	}
	if got[1].Passed {
		t.Errorf("test[1] should fail")
	}
	if got[1].Error == "" {
		t.Errorf("failing test should carry an error message")
	}
}

// TestAssertMatchers verifies the matcher subset and the .not negation.
func TestAssertMatchers(t *testing.T) {
	got := runTests(t, `
		test("toEqual", function () { expect(response.json).toEqual({ ok: true }); });
		test("toBeTruthy", function () { expect(response.json.ok).toBeTruthy(); });
		test("toContain", function () { expect(response.body).toContain("ok"); });
		test("toHaveLength", function () { expect([1,2,3]).toHaveLength(3); });
		test("toBeGreaterThan", function () { expect(response.status).toBeGreaterThan(199); });
		test("not.toBe", function () { expect(response.status).not.toBe(500); });
		test("toBeNull", function () { expect(null).toBeNull(); });
	`)
	if len(got) != 7 {
		t.Fatalf("recorded %d tests, want 7", len(got))
	}
	for _, tr := range got {
		if !tr.Passed {
			t.Errorf("matcher test %q failed: %s", tr.Name, tr.Error)
		}
	}
}

// TestAssertThrowInsideTestFails verifies a bare throw (not just a matcher)
// inside a test body is recorded as a failure.
func TestAssertThrowInsideTestFails(t *testing.T) {
	got := runTests(t, `test("boom", function () { throw new Error("kaboom"); });`)
	if len(got) != 1 || got[0].Passed || got[0].Error != "kaboom" {
		t.Errorf("throw-in-test = %+v, want one fail with 'kaboom'", got)
	}
}

// TestAssertAvailableInPreRequest verifies test()/expect() are also bound in
// the pre-request phase (so a script can assert on inputs).
func TestAssertAvailableInPreRequest(t *testing.T) {
	rt := New(newFakeBridge())
	r := model.Request{Method: model.GET, URL: "https://x/"}
	res, err := rt.RunPreRequest(context.Background(), `test("pre", function () { expect(1).toBe(1); });`, &r, nil)
	if err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if len(res.Tests) != 1 || !res.Tests[0].Passed {
		t.Errorf("pre-request test = %+v, want one pass", res.Tests)
	}
}
