package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/runner"
	"github.com/idct/helena/internal/storage"
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

// TestWriteReportFormats verifies the --format dispatch: json emits parseable
// JSON, junit emits parseable XML, and the default is the text summary.
func TestWriteReportFormats(t *testing.T) {
	rep := runner.Report{Results: []runner.RequestResult{
		{Path: "Health", Method: "GET", StatusCode: 200, Duration: time.Millisecond,
			Checks: []runner.Check{{Name: "status is 200", Passed: true}}},
	}}

	var jbuf bytes.Buffer
	if err := writeReport(&jbuf, rep, "json"); err != nil {
		t.Fatalf("writeReport json: %v", err)
	}
	if !json.Valid(jbuf.Bytes()) {
		t.Errorf("json format did not emit valid JSON:\n%s", jbuf.String())
	}

	var xbuf bytes.Buffer
	if err := writeReport(&xbuf, rep, "junit"); err != nil {
		t.Fatalf("writeReport junit: %v", err)
	}
	if err := xml.Unmarshal(xbuf.Bytes(), new(struct {
		XMLName xml.Name `xml:"testsuites"`
	})); err != nil {
		t.Errorf("junit format did not emit parseable <testsuites>: %v\n%s", err, xbuf.String())
	}

	var tbuf bytes.Buffer
	if err := writeReport(&tbuf, rep, "text"); err != nil {
		t.Fatalf("writeReport text: %v", err)
	}
	if !strings.Contains(tbuf.String(), "1 requests, 1 checks passed, 0 failed") {
		t.Errorf("text format missing summary:\n%s", tbuf.String())
	}
}

// TestRunCommandRejectsUnknownFormat verifies a bad --format fails fast (exit 2)
// before any collection is opened.
func TestRunCommandRejectsUnknownFormat(t *testing.T) {
	if code := runCommand([]string{"--format", "yaml", t.TempDir()}); code != 2 {
		t.Errorf("runCommand with --format yaml = %d, want 2", code)
	}
}

// TestRunCommandFlagAfterDir locks the two-phase parse: a flag written AFTER the
// collection dir (the README's `helena run ./col --format …` ordering) must be
// honored. A *valid, empty* collection is used so that if the flag were dropped
// the run would succeed (exit 0) — the bad --format only rejects (exit 2) when
// it is actually parsed.
func TestRunCommandFlagAfterDir(t *testing.T) {
	dir := t.TempDir()
	if err := storage.Save(model.Collection{Name: "Empty"}, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if code := runCommand([]string{dir, "--format", "yaml"}); code != 2 {
		t.Errorf("dir-first bad --format = %d, want 2 (flag after dir not parsed?)", code)
	}
	// A good format after the dir runs the (empty) collection cleanly.
	if code := runCommand([]string{dir, "--format", "json"}); code != 0 {
		t.Errorf("dir-first --format json on an empty collection = %d, want 0", code)
	}
}
