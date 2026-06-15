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

// reservedAliases are JavaScript reserved words. An alias is exposed as
// `chain.<alias>` and is also a natural binding name in scripts; rejecting
// reserved words keeps `chain.<alias>` from colliding with language keywords
// and surfaces a clear error at resolve time instead of a confusing script
// evaluation later.
var reservedAliases = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "enum": true, "export": true, "extends": true, "false": true,
	"finally": true, "for": true, "function": true, "if": true, "import": true,
	"in": true, "instanceof": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true,
	"with": true, "yield": true, "let": true, "static": true, "await": true,
}

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

// RequestFinder resolves a chain step's target — by stable Request.ID
// when the step carries one, falling back to the slash-separated name
// path otherwise. The production implementation lives on
// *session.SnapshotChainFinder; tests use a small map-backed fake.
//
// FindRequestByID returns false (without error) when no request in the
// snapshot carries the given ID; the chain runner then falls back to
// FindRequestByPath. An empty id MUST return false so steps without a
// pinned ID skip the ID lookup cleanly.
type RequestFinder interface {
	FindRequestByPath(ref string) (model.Request, bool)
	FindRequestByID(id string) (model.Request, bool)
}

// ProgressFunc is invoked once at the start of each chain step's
// ExecuteOnce call. step is 1-based across the whole Resolve; total
// is the upfront-counted total (so callers can show "step 2/3").
// alias is the step's declared alias in its parent's chain; name is
// the resolved request's Name.
//
// Progress always runs on the goroutine calling Resolve. UI callers
// must marshal to the UI thread (e.g. via fyne.Do) — the chain runner
// is invariant 4 safe by virtue of not touching widgets itself, but
// any UI work the callback does is not.
type ProgressFunc func(step, total int, alias, name string)

// Resolve executes every before-hook of leaf, recursively, and returns
// the alias→View map the leaf's own scripts should see — plus the
// accumulated console output from every step run so far. The leaf
// itself is NOT executed here; the caller (the UI Send pipeline) runs
// it via the same Executor with the returned map.
//
// progress (may be nil) is called once before each chain step's
// ExecuteOnce so callers can show "step N/total" feedback during
// multi-step chains. When non-nil, Resolve does a single upfront
// finder walk to compute the total — cheap (bounded by MaxChainSteps)
// and identical to the execution-time walk so the total matches what
// will actually run.
//
// Returns an error if any chain step fails to execute, if any step
// references an unknown request path, if a cycle is detected, or if
// the chain exceeds MaxChainDepth / MaxChainSteps. The error names the
// offending alias / path so the user can fix it.
func Resolve(ctx context.Context, leaf model.Request, finder RequestFinder, exec Executor, progress ProgressFunc) (map[string]View, []string, error) {
	visiting := map[string]bool{}
	if leaf.ID != "" {
		visiting[leaf.ID] = true
	}
	total := 0
	if progress != nil {
		countVisiting := map[string]bool{}
		if leaf.ID != "" {
			countVisiting[leaf.ID] = true
		}
		total = countSteps(leaf.Chain, finder, countVisiting, 0)
	}
	var console []string
	stepCount := 0
	chainMap, err := resolveSteps(ctx, leaf.Chain, finder, exec, visiting, &console, 0, &stepCount, progress, total)
	return chainMap, console, err
}

// resolveSteps expands the supplied chain entries for one request,
// executing each step (and its own chain transitively) before the
// next. The visiting set is shared across the recursion so cycles
// anywhere in the graph are caught. depth is the current recursion
// level (0 = leaf's direct chain); stepCount tallies total
// ExecuteOnce calls across the whole Resolve and is shared via
// pointer so caps apply globally.
func resolveSteps(ctx context.Context, steps []model.ChainStep, finder RequestFinder, exec Executor, visiting map[string]bool, console *[]string, depth int, stepCount *int, progress ProgressFunc, total int) (map[string]View, error) {
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
		if reservedAliases[step.Alias] {
			return nil, fmt.Errorf("chain: alias %q is a reserved JavaScript word; pick another name", step.Alias)
		}
		if _, dup := out[step.Alias]; dup {
			return nil, fmt.Errorf("chain: duplicate alias %q in the same request's chain", step.Alias)
		}
		sub, ok := resolveTarget(finder, step)
		if !ok {
			return nil, fmt.Errorf("chain: cannot resolve request %q (alias %q)", step.Request, step.Alias)
		}
		key := visitKey(sub, step)
		if visiting[key] {
			return nil, fmt.Errorf("chain: cycle detected through %q (alias %q)", sub.Name, step.Alias)
		}
		visiting[key] = true
		subChainMap, err := resolveSteps(ctx, sub.Chain, finder, exec, visiting, console, depth+1, stepCount, progress, total)
		if err != nil {
			return nil, err
		}
		if *stepCount >= MaxChainSteps {
			return nil, fmt.Errorf("chain: step count exceeds limit %d (likely a fan-out attempt)", MaxChainSteps)
		}
		*stepCount++
		if progress != nil {
			progress(*stepCount, total, step.Alias, sub.Name)
		}
		view, lines, err := exec.ExecuteOnce(ctx, sub, subChainMap)
		appendBounded(console, lines)
		// Trace what actually went out for each chain step. The HTTP
		// only ran (view.Request.URL non-empty) when pre-script + HTTP
		// both succeeded, so this captures the post-pre-script wire
		// URL — the value a leaf script debugging the chain would need
		// to see. Errors are still surfaced separately.
		if view.Request.URL != "" {
			appendBounded(console, []string{fmt.Sprintf("→ chain[%s] %s %s", step.Alias, view.Request.Method, view.Request.URL)})
		}
		if err != nil {
			return nil, fmt.Errorf("chain step %q (%s): %w", step.Alias, sub.Name, err)
		}
		delete(visiting, key)
		out[step.Alias] = view
	}
	return out, nil
}

// visitKey returns the cycle-detection key for a resolved chain target. It uses
// the stable Request.ID when present, and otherwise falls back to a path-derived
// sentinel key. Without the fallback an empty-ID request — the state of any
// freshly imported or never-saved collection, since IDs are backfilled only on
// the first Helena save (invariant 14) — would skip the visiting set entirely
// and a self/mutual reference would recurse until the depth/step caps instead of
// being reported as a cycle. The NUL prefix keeps the path key from colliding
// with a real ID.
func visitKey(sub model.Request, step model.ChainStep) string {
	if sub.ID != "" {
		return sub.ID
	}
	return "\x00path:" + step.Request
}

// resolveTarget looks up a chain step's target via the finder. It
// prefers the step's RequestID (a stable identifier that survives
// renames + folder moves) and falls back to the human-readable Request
// path when the ID is empty or not found in the snapshot. Returning
// false leaves the caller to surface the standard "cannot resolve
// request" error against the path field (the user-visible
// identifier).
func resolveTarget(finder RequestFinder, step model.ChainStep) (model.Request, bool) {
	if step.RequestID != "" {
		if r, ok := finder.FindRequestByID(step.RequestID); ok {
			return r, true
		}
	}
	if step.Request == "" {
		return model.Request{}, false
	}
	return finder.FindRequestByPath(step.Request)
}

// countSteps mirrors the resolveSteps walk to pre-count how many
// ExecuteOnce calls a Resolve will issue. Used only when a progress
// callback was supplied — lets callers display "step N/total" with
// total known upfront rather than reported step-at-a-time.
//
// Unresolvable refs are silently skipped (the actual Resolve will
// error on them); cycles are skipped via the shared visiting map so
// the count matches what Resolve will run. The total is capped at
// MaxChainSteps for consistency with the runtime cap.
func countSteps(steps []model.ChainStep, finder RequestFinder, visiting map[string]bool, depth int) int {
	if depth >= MaxChainDepth {
		return 0
	}
	total := 0
	for _, step := range steps {
		if step.Request == "" && step.RequestID == "" {
			continue
		}
		sub, ok := resolveTarget(finder, step)
		if !ok {
			continue
		}
		key := visitKey(sub, step)
		if visiting[key] {
			continue
		}
		visiting[key] = true
		total += countSteps(sub.Chain, finder, visiting, depth+1)
		total++
		delete(visiting, key)
		if total >= MaxChainSteps {
			return MaxChainSteps
		}
	}
	return total
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
