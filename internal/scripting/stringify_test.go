package scripting

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestStringifyUnmarshalableFallsBackToJSString verifies that console.log of a
// value json can't marshal (a function) yields a stable, JS-shaped string
// rather than Go's %v (which leaks pointer addresses and varies per run)
// (regression for #21).
func TestStringifyUnmarshalableFallsBackToJSString(t *testing.T) {
	vm := goja.New()
	v, err := vm.RunString("(function add(a, b) { return a + b; })")
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := stringify(v)
	if strings.Contains(got, "0x") {
		t.Errorf("stringify leaked a Go pointer address: %q", got)
	}
	if !strings.Contains(got, "function") {
		t.Errorf("stringify(function) = %q, want a JS function source string", got)
	}
}
