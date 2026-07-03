package scripting

import (
	"sync"

	"github.com/dop251/goja"
)

// Compiled-program cache. A Runtime (and its goja VM) is created per run for
// isolation, but the same pre/post script text is recompiled on every send —
// and twice per chain step. goja.Program is immutable and explicitly safe to
// run on any number of runtimes, so each distinct source is compiled once and
// shared process-wide. Compiling with name "" matches vm.RunString, so error
// positions are unchanged; the error is now the bare *goja.CompilerSyntaxError
// instead of RunString's JS-exception wrapping (which doubled the
// "SyntaxError:" prefix). Only successful compiles are cached; a script with
// a syntax error re-reports on every run.
const maxCachedPrograms = 128

var (
	progMu   sync.Mutex
	programs = map[string]*goja.Program{}
)

// compileCached returns the shared compiled program for src, compiling on
// first sight. At the (generous) cap the whole cache is dropped rather than
// grown — simpler than an LRU, and only pathological generated-script churn
// ever hits it.
func compileCached(src string) (*goja.Program, error) {
	progMu.Lock()
	p, ok := programs[src]
	progMu.Unlock()
	if ok {
		return p, nil
	}
	p, err := goja.Compile("", src, false)
	if err != nil {
		return nil, err
	}
	progMu.Lock()
	if len(programs) >= maxCachedPrograms {
		clear(programs)
	}
	programs[src] = p
	progMu.Unlock()
	return p, nil
}
