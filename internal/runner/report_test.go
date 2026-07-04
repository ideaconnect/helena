package runner

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

// sampleReport is a run with one of each outcome: an all-pass request, a
// failed-check request, an errored request, and a script-skipped request.
func sampleReport() Report {
	return Report{Results: []RequestResult{
		{Path: "Health", Method: "GET", URL: "https://x/health", StatusCode: 200, Duration: 2 * time.Millisecond,
			Checks: []Check{{Name: "status is 200", Passed: true}}},
		{Path: "Sub/Broken", Method: "POST", URL: "https://x/broken", StatusCode: 500, Duration: 5 * time.Millisecond,
			Checks: []Check{{Name: "status is 200", Passed: false, Error: `expected "500" to equal "200"`}}},
		{Path: "Down", Method: "GET", Duration: time.Millisecond, Err: "dial tcp: connection refused"},
		{Path: "Optional", Method: "GET", Skipped: true, Duration: 0},
	}}
}

// jsonReportMirror mirrors the (unexported) JSON shape so the test can decode
// and assert on the real emitted bytes.
type jsonReportMirror struct {
	Requests     int  `json:"requests"`
	ChecksPassed int  `json:"checksPassed"`
	ChecksFailed int  `json:"checksFailed"`
	Failed       bool `json:"failed"`
	Results      []struct {
		Path       string  `json:"path"`
		Method     string  `json:"method"`
		URL        string  `json:"url"`
		Status     int     `json:"status"`
		DurationMs float64 `json:"durationMs"`
		OK         bool    `json:"ok"`
		Skipped    bool    `json:"skipped"`
		Error      string  `json:"error"`
		Checks     []struct {
			Name   string `json:"name"`
			Passed bool   `json:"passed"`
			Error  string `json:"error"`
		} `json:"checks"`
	} `json:"results"`
}

func TestReportJSON(t *testing.T) {
	b, err := sampleReport().JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if b[len(b)-1] != '\n' {
		t.Errorf("JSON output should end with a newline")
	}
	var got jsonReportMirror
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("emitted JSON does not parse: %v\n%s", err, b)
	}
	if got.Requests != 4 || got.ChecksPassed != 1 || got.ChecksFailed != 1 {
		t.Errorf("totals = %d req / %d pass / %d fail, want 4/1/1", got.Requests, got.ChecksPassed, got.ChecksFailed)
	}
	if !got.Failed {
		t.Errorf("failed=false, want true (a check failed and a request errored)")
	}
	if len(got.Results) != 4 {
		t.Fatalf("got %d results, want 4", len(got.Results))
	}
	// Health: ok, duration rendered in ms, URL carried through.
	if r := got.Results[0]; !r.OK || r.DurationMs != 2 || r.URL != "https://x/health" {
		t.Errorf("Health result = %+v, want ok/2ms/url", r)
	}
	// Broken: not ok, check error carried through.
	if r := got.Results[1]; r.OK || len(r.Checks) != 1 || r.Checks[0].Passed || r.Checks[0].Error == "" {
		t.Errorf("Broken result = %+v, want not-ok with a failed check + error", r)
	}
	// Down: errored request.
	if r := got.Results[2]; r.OK || r.Error == "" {
		t.Errorf("Down result = %+v, want not-ok with an error string", r)
	}
	// Optional: skipped is OK.
	if r := got.Results[3]; !r.Skipped || !r.OK {
		t.Errorf("Optional result = %+v, want skipped + ok", r)
	}
}

// junitMirror mirrors the emitted JUnit XML for assertions.
type junitMirror struct {
	XMLName  xml.Name `xml:"testsuites"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Skipped  int      `xml:"skipped,attr"`
	Suite    struct {
		Name  string `xml:"name,attr"`
		Cases []struct {
			Name      string `xml:"name,attr"`
			Classname string `xml:"classname,attr"`
			Failure   *struct {
				Message string `xml:"message,attr"`
				Body    string `xml:",chardata"`
			} `xml:"failure"`
			Skipped *struct{} `xml:"skipped"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

func TestReportJUnit(t *testing.T) {
	b, err := sampleReport().JUnit()
	if err != nil {
		t.Fatalf("JUnit: %v", err)
	}
	if !strings.HasPrefix(string(b), xml.Header) {
		t.Errorf("JUnit output should start with the XML header")
	}
	var got junitMirror
	if err := xml.Unmarshal(b, &got); err != nil {
		t.Fatalf("emitted JUnit does not parse: %v\n%s", err, b)
	}
	if got.Tests != 4 || got.Failures != 2 || got.Skipped != 1 {
		t.Errorf("root counts = %d tests / %d failures / %d skipped, want 4/2/1", got.Tests, got.Failures, got.Skipped)
	}
	if len(got.Suite.Cases) != 4 {
		t.Fatalf("got %d testcases, want 4", len(got.Suite.Cases))
	}
	// Health passes: no <failure>, no <skipped>.
	if c := got.Suite.Cases[0]; c.Failure != nil || c.Skipped != nil || c.Classname != "GET" {
		t.Errorf("Health case = %+v, want a clean pass with classname GET", c)
	}
	// Broken: a failure whose body names the failed check.
	if c := got.Suite.Cases[1]; c.Failure == nil || !strings.Contains(c.Failure.Body, "status is 200") {
		t.Errorf("Broken case = %+v, want a <failure> mentioning the check", c)
	}
	// Down: a failure whose message is the execution error.
	if c := got.Suite.Cases[2]; c.Failure == nil || !strings.Contains(c.Failure.Message, "connection refused") {
		t.Errorf("Down case = %+v, want a <failure> with the error message", c)
	}
	// Optional: skipped element present.
	if c := got.Suite.Cases[3]; c.Skipped == nil || c.Failure != nil {
		t.Errorf("Optional case = %+v, want <skipped/>", c)
	}
}

// TestReportJUnitSkippedButFailedIsFailure guards the precedence: a request that
// was marked skipped but still failed a check before skipping must render as a
// failure, not a skip.
func TestReportJUnitSkippedButFailedIsFailure(t *testing.T) {
	rep := Report{Results: []RequestResult{
		{Path: "Weird", Method: "GET", Skipped: true,
			Checks: []Check{{Name: "pre-check", Passed: false, Error: "boom"}}},
	}}
	b, err := rep.JUnit()
	if err != nil {
		t.Fatalf("JUnit: %v", err)
	}
	var got junitMirror
	if err := xml.Unmarshal(b, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Failures != 1 || got.Skipped != 0 {
		t.Errorf("counts = %d fail / %d skip, want 1/0 (failed check outranks skip)", got.Failures, got.Skipped)
	}
	if c := got.Suite.Cases[0]; c.Failure == nil || c.Skipped != nil {
		t.Errorf("case = %+v, want <failure>, not <skipped/>", c)
	}
}

// TestReportJSONEmpty: a run with no requests still emits valid JSON with zeroed
// totals and failed=false (nothing failed).
func TestReportJSONEmpty(t *testing.T) {
	b, err := Report{}.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var got jsonReportMirror
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Requests != 0 || got.Failed || len(got.Results) != 0 {
		t.Errorf("empty report = %+v, want 0 requests / failed=false / no results", got)
	}
}
