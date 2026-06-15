package scripting

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestMergeKVFromObjectDeterministicOrder verifies script-added headers/params
// are emitted in the JS object's insertion order, deterministically across runs
// — not Go map-iteration order (regression for #103).
func TestMergeKVFromObjectDeterministicOrder(t *testing.T) {
	vm := goja.New()
	build := func() *goja.Object {
		o := vm.NewObject()
		for _, k := range []string{"Zeta", "Alpha", "Mu", "Beta", "Gamma"} {
			_ = o.Set(k, "v")
		}
		return o
	}
	const want = "Zeta,Alpha,Mu,Beta,Gamma"
	for iter := 0; iter < 25; iter++ {
		merged := mergeKVFromObject(nil, build())
		var keys []string
		for _, kv := range merged {
			keys = append(keys, kv.Key)
		}
		if got := strings.Join(keys, ","); got != want {
			t.Fatalf("iter %d: script-added order = %q, want %q", iter, got, want)
		}
	}
}
