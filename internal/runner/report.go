package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

// Machine-readable renderings of a run Report (#90), for CI consumption:
// `helena run --format json` and `--format junit`. Both are pure functions of
// the Report — no I/O — so they are unit-tested here in the gated package; the
// CLI (cmd/helena) only selects one and writes the bytes.

// --- JSON ---

// jsonReport is the stable JSON shape of a run. It is a dedicated DTO rather
// than json tags on RequestResult so the wire format stays decoupled from the
// internal types (e.g. Duration is emitted as milliseconds, not raw ns).
type jsonReport struct {
	Requests     int          `json:"requests"`
	ChecksPassed int          `json:"checksPassed"`
	ChecksFailed int          `json:"checksFailed"`
	Failed       bool         `json:"failed"`
	Results      []jsonResult `json:"results"`
}

type jsonResult struct {
	Path       string      `json:"path"`
	Method     string      `json:"method"`
	URL        string      `json:"url,omitempty"`
	Status     int         `json:"status"`
	DurationMs float64     `json:"durationMs"`
	OK         bool        `json:"ok"`
	Skipped    bool        `json:"skipped,omitempty"`
	Error      string      `json:"error,omitempty"`
	Checks     []jsonCheck `json:"checks,omitempty"`
}

type jsonCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

// JSON renders the report as indented JSON with a trailing newline. The top
// level carries the same totals as the text summary plus a `failed` boolean
// mirroring the process exit code, so a CI step can branch on the report alone.
func (rp Report) JSON() ([]byte, error) {
	reqs, passed, failed := rp.Totals()
	out := jsonReport{
		Requests:     reqs,
		ChecksPassed: passed,
		ChecksFailed: failed,
		Failed:       rp.Failed(),
		Results:      make([]jsonResult, 0, len(rp.Results)),
	}
	for _, r := range rp.Results {
		jr := jsonResult{
			Path:       r.Path,
			Method:     r.Method,
			URL:        r.URL,
			Status:     r.StatusCode,
			DurationMs: float64(r.Duration.Microseconds()) / 1000,
			OK:         r.OK(),
			Skipped:    r.Skipped,
			Error:      r.Err,
		}
		for _, c := range r.Checks {
			// Check and jsonCheck are structurally identical (same fields/types),
			// so a conversion suffices — the json tags live on jsonCheck (gosimple
			// S1016).
			jr.Checks = append(jr.Checks, jsonCheck(c))
		}
		out.Results = append(out.Results, jr)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// --- JUnit XML ---

type junitTestsuites struct {
	XMLName  xml.Name       `xml:"testsuites"`
	Tests    int            `xml:"tests,attr"`
	Failures int            `xml:"failures,attr"`
	Skipped  int            `xml:"skipped,attr"`
	Suite    junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Cases    []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct{}

// JUnit renders the report as JUnit XML — one <testcase> per request (name =
// tree path, classname = method, time in seconds). An errored request or one
// with a failed check becomes a <failure> (message = summary, body = detail); a
// script-skipped request becomes <skipped/>. The suite/root counters let a CI
// dashboard tally without walking the cases.
func (rp Report) JUnit() ([]byte, error) {
	suite := junitTestsuite{Name: "helena"}
	for _, r := range rp.Results {
		tc := junitTestcase{
			Name:      r.Path,
			Classname: r.Method,
			Time:      r.Duration.Seconds(),
		}
		switch {
		case r.Skipped && r.OK():
			tc.Skipped = &junitSkipped{}
			suite.Skipped++
		case !r.OK():
			tc.Failure = &junitFailure{Message: failureMessage(r), Content: failureDetail(r)}
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, tc)
		suite.Tests++
	}
	doc := junitTestsuites{
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Skipped:  suite.Skipped,
		Suite:    suite,
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	out := append([]byte(xml.Header), b...)
	return append(out, '\n'), nil
}

// failureMessage is the one-line <failure message> for a non-OK request: the
// execution error if any, else a summary of which checks failed.
func failureMessage(r RequestResult) string {
	if r.Err != "" {
		return r.Err
	}
	var failed []string
	for _, c := range r.Checks {
		if !c.Passed {
			failed = append(failed, c.Name)
		}
	}
	if len(failed) > 0 {
		return fmt.Sprintf("%d check(s) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return "failed"
}

// failureDetail is the multi-line <failure> body: the error and every failed
// check with its own message.
func failureDetail(r RequestResult) string {
	var b strings.Builder
	if r.Err != "" {
		fmt.Fprintf(&b, "error: %s\n", r.Err)
	}
	for _, c := range r.Checks {
		if c.Passed {
			continue
		}
		if c.Error != "" {
			fmt.Fprintf(&b, "%s: %s\n", c.Name, c.Error)
		} else {
			fmt.Fprintf(&b, "%s: failed\n", c.Name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
