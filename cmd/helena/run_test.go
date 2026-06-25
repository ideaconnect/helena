package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/idct/helena/internal/runner"
)

// TestPrintReport verifies the report renders an ok line, a FAIL line with its
// check error, and a totals summary.
func TestPrintReport(t *testing.T) {
	rep := runner.Report{Results: []runner.RequestResult{
		{Path: "Health", Method: "GET", StatusCode: 200, Duration: time.Millisecond,
			Checks: []runner.Check{{Name: "res.status equals 200", Passed: true}}},
		{Path: "Sub/Broken", Method: "GET", StatusCode: 500, Duration: time.Millisecond,
			Checks: []runner.Check{{Name: "res.status equals 200", Passed: false, Error: `expected "500" to equal "200"`}}},
	}}
	var buf bytes.Buffer
	printReport(&buf, rep)
	out := buf.String()

	for _, want := range []string{
		"ok    Health", "FAIL  Sub/Broken",
		`FAIL  res.status equals 200 — expected "500" to equal "200"`,
		"2 requests, 1 checks passed, 1 failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}
