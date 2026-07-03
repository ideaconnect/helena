package scripting

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/idct/helena/internal/model"
)

// TestCompileCachedSharesPrograms: one distinct source compiles once and the
// same *goja.Program is handed to every later run; distinct sources get
// distinct programs.
func TestCompileCachedSharesPrograms(t *testing.T) {
	p1, err := compileCached("1 + 1")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	p2, err := compileCached("1 + 1")
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if p1 != p2 {
		t.Error("same source should return the cached program instance")
	}
	p3, err := compileCached("2 + 2")
	if err != nil {
		t.Fatalf("compile other: %v", err)
	}
	if p3 == p1 {
		t.Error("distinct sources must not share a program")
	}
}

// TestCompileCachedProgramRunsOnFreshRuntimes: the shared program must run
// correctly on multiple independent runtimes (the per-send isolation model).
func TestCompileCachedProgramRunsOnFreshRuntimes(t *testing.T) {
	prog, err := compileCached("40 + 2")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for i := 0; i < 2; i++ {
		vm := goja.New()
		v, err := vm.RunProgram(prog)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if v.ToInteger() != 42 {
			t.Fatalf("run %d = %v, want 42", i, v)
		}
	}
}

// TestCompileCachedSyntaxErrorNotCached: failed compiles are reported every
// time and never enter the cache.
func TestCompileCachedSyntaxErrorNotCached(t *testing.T) {
	if _, err := compileCached("this is (not js"); err == nil {
		t.Fatal("expected a syntax error")
	}
	progMu.Lock()
	_, cached := programs["this is (not js"]
	progMu.Unlock()
	if cached {
		t.Error("a failed compile must not be cached")
	}
}

// TestCompileCachedCapResets: at the cap the cache resets instead of growing
// without bound.
func TestCompileCachedCapResets(t *testing.T) {
	progMu.Lock()
	clear(programs)
	progMu.Unlock()
	for i := 0; i < maxCachedPrograms; i++ {
		if _, err := compileCached("void " + string(rune('a'+i%26)) + "; 1e" + itoa(i)); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	if _, err := compileCached("'overflow'"); err != nil {
		t.Fatal(err)
	}
	progMu.Lock()
	n := len(programs)
	progMu.Unlock()
	if n != 1 {
		t.Errorf("cache size after overflow = %d, want 1 (reset + the new entry)", n)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// BenchmarkRunPreRequestRepeatedScript measures the per-send script cost for
// a request re-sent with the same pre-script — the case the program cache
// targets (previously the script recompiled on every run).
func BenchmarkRunPreRequestRepeatedScript(b *testing.B) {
	rt := New(nil)
	script := `
		const parts = [];
		for (let i = 0; i < 50; i++) { parts.push("h-" + i); }
		request.headers["X-Bench"] = parts.join(",");
	`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := model.Request{Method: model.GET, URL: "https://x/y"}
		if _, err := rt.RunPreRequest(context.Background(), script, &r, nil); err != nil {
			b.Fatal(err)
		}
	}
}
