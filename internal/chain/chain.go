// Package chain executes per-request before-hooks ("chains"): when a
// request declares `Chain []ChainStep`, each named predecessor is
// executed in order before the leaf request runs. The leaf's own
// scripts then see each predecessor's response via the `chain.<alias>`
// global supplied by [internal/scripting].
//
// Resolution is recursive: a chain step's own Chain expands first, so
// running A → [B] → [C] produces the order C, B, A. Each request's
// chain map is private to its own scripts — A does not see C's alias,
// only B's. Cycles are detected by tracking the currently-resolving
// request IDs and surfaced as a clear error rather than a stack
// overflow.
//
// The package stays decoupled from [internal/httpclient] and
// [internal/ui] behind the Executor and RequestFinder interfaces, so
// the chain runner can be exercised in tests with fakes for both.
package chain

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/idct/helena/internal/model"
)

// MaxChainDepth caps the recursion depth of chain expansion. A
// pathological imported collection could otherwise declare an arbitrarily
// deep linear chain (A → B → C → … → Z) and turn one user click into
// dozens of serial HTTP calls.
const MaxChainDepth = 8

// MaxChainSteps caps the total number of ExecuteOnce calls a single
// chain.Resolve can issue. Together with MaxChainDepth this bounds the
// fan-out a hostile YAML can trigger (an alphabet-spanning A → [B1..Bn]
// with each Bi → [C1..Cn] otherwise fans into n*n requests).
const MaxChainSteps = 32

// MaxChainConsoleLines is the soft cap on console output the runner
// accumulates across every chain step. Past this point further lines
// are dropped and a single "[chain console truncated]" marker is added.
const MaxChainConsoleLines = 1024

// aliasRe matches a JS identifier — what dotted access `chain.<alias>`
// supports without quoting.
var aliasRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// View is the snapshot of one executed request — what its sibling
// chain steps and the leaf will see via `chain.<alias>.{request,response}`.
// Body and Headers are copies; mutating them in a script doesn't
// affect future requests.
type View struct {
	Request  RequestView
	Response ResponseView
}

// RequestView mirrors the static fields scripts care about. The body
// is the post-resolution wire body; URL is the final URL with query
// params merged.
type RequestView struct {
	Method string
	URL    string
	Body   []byte
}

// ResponseView mirrors the response surface available to scripts plus
// the display-only fields the UI uses to render the response panel.
// Size is len(Body) — kept explicit so callers don't compute it twice.
// CORSWarning is only meaningful for the leaf request the user
// initiated, but storing it here keeps the leaf and chain-step display
// paths uniform.
type ResponseView struct {
	StatusCode  int
	Status      string
	Headers     http.Header
	Body        []byte
	Size        int64
	Duration    time.Duration
	CORSWarning string
}

// Executor runs a single resolved request through pre-script, HTTP,
// and post-script with the given chainMap bound as the script-side
// `chain` global. Returns the captured View, any console lines emitted
// during pre or post scripts (so the UI can show the full trace), and
// the first error from any of the three stages.
//
// The chain runner calls this once per chain step plus once for the
// leaf; the leaf's own scripts are run by ExecuteOnce too, so the
// leaf-side caller never needs a separate code path for "request with
// no chain" — Resolve simply returns an empty chainMap in that case.
type Executor interface {
	ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]View) (View, []string, error)
}

// RequestFinder resolves a chain step's `Request` field — a
// slash-separated name path — into the actual model.Request. The
// production implementation lives on *session.Session
// (FindRequestByPath); tests use a small map-backed fake.
type RequestFinder interface {
	FindRequestByPath(ref string) (model.Request, bool)
}

// Resolve executes every before-hook of leaf, recursively, and returns
// the alias→View map the leaf's own scripts should see — plus the
// accumulated console output from every step run so far. The leaf
// itself is NOT executed here; the caller (the UI Send pipeline) runs
// it via the same Executor with the returned map.
//
// Returns an error if any chain step fails to execute, if any step
// references an unknown request path, if a cycle is detected, or if
// the chain exceeds MaxChainDepth / MaxChainSteps. The error names the
// offending alias / path so the user can fix it.
func Resolve(ctx context.Context, leaf model.Request, finder RequestFinder, exec Executor) (map[string]View, []string, error) {
	visiting := map[string]bool{}
	if leaf.ID != "" {
		visiting[leaf.ID] = true
	}
	var console []string
	stepCount := 0
	chainMap, err := resolveSteps(ctx, leaf.Chain, finder, exec, visiting, &console, 0, &stepCount)
	return chainMap, console, err
}

// resolveSteps expands the supplied chain entries for one request,
// executing each step (and its own chain transitively) before the
// next. The visiting set is shared across the recursion so cycles
// anywhere in the graph are caught. depth is the current recursion
// level (0 = leaf's direct chain); stepCount tallies total
// ExecuteOnce calls across the whole Resolve and is shared via
// pointer so caps apply globally.
func resolveSteps(ctx context.Context, steps []model.ChainStep, finder RequestFinder, exec Executor, visiting map[string]bool, console *[]string, depth int, stepCount *int) (map[string]View, error) {
	if depth >= MaxChainDepth {
		return nil, fmt.Errorf("chain: depth exceeds limit %d (likely a runaway nested chain)", MaxChainDepth)
	}
	out := make(map[string]View, len(steps))
	for _, step := range steps {
		blankAlias := step.Alias == ""
		blankRef := step.Request == ""
		if blankAlias && blankRef {
			return nil, fmt.Errorf("chain: a step is missing both alias and request reference")
		}
		if blankAlias {
			return nil, fmt.Errorf("chain: step is missing an alias (request %q)", step.Request)
		}
		if blankRef {
			return nil, fmt.Errorf("chain: alias %q has no request reference", step.Alias)
		}
		if !aliasRe.MatchString(step.Alias) {
			return nil, fmt.Errorf("chain: alias %q is not a valid JS identifier (use letters, digits, _, $; must not start with a digit)", step.Alias)
		}
		if _, dup := out[step.Alias]; dup {
			return nil, fmt.Errorf("chain: duplicate alias %q in the same request's chain", step.Alias)
		}
		sub, ok := finder.FindRequestByPath(step.Request)
		if !ok {
			return nil, fmt.Errorf("chain: cannot resolve request %q (alias %q)", step.Request, step.Alias)
		}
		if sub.ID != "" && visiting[sub.ID] {
			return nil, fmt.Errorf("chain: cycle detected through %q (alias %q)", sub.Name, step.Alias)
		}
		if sub.ID != "" {
			visiting[sub.ID] = true
		}
		subChainMap, err := resolveSteps(ctx, sub.Chain, finder, exec, visiting, console, depth+1, stepCount)
		if err != nil {
			return nil, err
		}
		if *stepCount >= MaxChainSteps {
			return nil, fmt.Errorf("chain: step count exceeds limit %d (likely a fan-out attempt)", MaxChainSteps)
		}
		*stepCount++
		view, lines, err := exec.ExecuteOnce(ctx, sub, subChainMap)
		appendBounded(console, lines)
		if err != nil {
			return nil, fmt.Errorf("chain step %q (%s): %w", step.Alias, sub.Name, err)
		}
		if sub.ID != "" {
			delete(visiting, sub.ID)
		}
		out[step.Alias] = view
	}
	return out, nil
}

// appendBounded appends lines to *console, dropping any past
// MaxChainConsoleLines and inserting a single truncation marker the
// first time the cap is hit. Keeps the cumulative chain console
// bounded even when individual scripts emit gigabytes of logs.
func appendBounded(console *[]string, lines []string) {
	for _, l := range lines {
		if len(*console) >= MaxChainConsoleLines {
			if len(*console) == MaxChainConsoleLines {
				*console = append(*console, "[chain console truncated]")
			}
			return
		}
		*console = append(*console, l)
	}
}
