package ui

import (
	"reflect"
	"testing"

	"github.com/idct/helena/internal/scripting"
)

// TestFormatTestResults verifies pass/fail lines and the summary (#87).
func TestFormatTestResults(t *testing.T) {
	got := formatTestResults([]scripting.TestResult{
		{Name: "status is 200", Passed: true},
		{Name: "has token", Passed: false, Error: "expected undefined to be defined"},
	})
	want := []string{
		"PASS  status is 200",
		"FAIL  has token — expected undefined to be defined",
		"Tests: 1 passed, 1 failed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("formatTestResults =\n%v\nwant\n%v", got, want)
	}
}
