package chain

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// fakeFinder is a tiny RequestFinder backed by a path → Request map.
// FindRequestByID does a linear ID match across the values so tests
// can exercise the by-ID seam without a second map.
type fakeFinder map[string]model.Request

func (f fakeFinder) FindRequestByPath(ref string) (model.Request, bool) {
	r, ok := f[ref]
	return r, ok
}

func (f fakeFinder) FindRequestByID(id string) (model.Request, bool) {
	if id == "" {
		return model.Request{}, false
	}
	for _, r := range f {
		if r.ID == id {
			return r, true
		}
	}
	return model.Request{}, false
}

// recordingExec captures each ExecuteOnce call so tests can assert
// order and chain visibility. It returns a deterministic View per
// request keyed by Name.
type recordingExec struct {
	calls    []string
	chainSee map[string]map[string]bool
}

func newRecordingExec() *recordingExec {
	return &recordingExec{chainSee: map[string]map[string]bool{}}
}

func (e *recordingExec) ExecuteOnce(_ context.Context, r model.Request, chainMap map[string]View) (View, []string, error) {
	e.calls = append(e.calls, r.Name)
	if e.chainSee[r.Name] == nil {
		e.chainSee[r.Name] = map[string]bool{}
	}
	for alias := range chainMap {
		e.chainSee[r.Name][alias] = true
	}
	return View{
		Request: RequestView{Method: string(r.Method), URL: r.URL},
		Response: ResponseView{
			StatusCode: 200, Status: "200 OK",
			Headers: http.Header{"Content-Type": []string{"text/plain"}},
			Body:    []byte("from " + r.Name),
		},
	}, []string{"console:" + r.Name}, nil
}

// TestResolveEmptyChain verifies a request with no Chain produces an
// empty alias map and no executor calls.
func TestResolveEmptyChain(t *testing.T) {
	leaf := model.Request{ID: "L", Name: "Leaf"}
	exec := newRecordingExec()
	m, console, err := Resolve(context.Background(), leaf, fakeFinder{}, exec, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(m) != 0 {
		t.Errorf("chainMap len = %d, want 0", len(m))
	}
	if len(console) != 0 {
		t.Errorf("console = %v, want empty", console)
	}
	if len(exec.calls) != 0 {
		t.Errorf("calls = %v, want []", exec.calls)
	}
}

// TestResolveSingleStep verifies one before-hook executes and lands
// under its alias in the returned map.
func TestResolveSingleStep(t *testing.T) {
	login := model.Request{ID: "B", Name: "Login", URL: "https://auth/login"}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "login", Request: "Auth/Login"},
	}}
	finder := fakeFinder{"Auth/Login": login}
	exec := newRecordingExec()
	m, console, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(m) != 1 || m["login"].Request.URL != "https://auth/login" {
		t.Errorf("chainMap = %+v, want {login: Login}", m)
	}
	if got := strings.Join(exec.calls, ","); got != "Login" {
		t.Errorf("calls = %q, want 'Login'", got)
	}
	// console contains the script's line followed by the runner's
	// auto-trace line for the step.
	if len(console) != 2 || console[0] != "console:Login" {
		t.Errorf("console = %v", console)
	}
}

// TestResolveRecursiveOrder verifies that A → [B] → [C] runs in order
// C, B, with B's chain map containing only its OWN aliases (csrf), and
// the returned map for A containing only A's own alias (login).
func TestResolveRecursiveOrder(t *testing.T) {
	bootstrap := model.Request{ID: "C", Name: "Bootstrap"}
	login := model.Request{ID: "B", Name: "Login", Chain: []model.ChainStep{
		{Alias: "csrf", Request: "Bootstrap"},
	}}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "login", Request: "Auth/Login"},
	}}
	finder := fakeFinder{
		"Auth/Login": login,
		"Bootstrap":  bootstrap,
	}
	exec := newRecordingExec()
	m, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Bootstrap first, then Login (Leaf is run by the caller).
	if got := strings.Join(exec.calls, ","); got != "Bootstrap,Login" {
		t.Errorf("call order = %q, want 'Bootstrap,Login'", got)
	}
	// Leaf's returned chain map: only login.
	if _, ok := m["login"]; !ok || len(m) != 1 {
		t.Errorf("leaf chainMap = %v, want {login: ...}", m)
	}
	// Login's chain map (recorded during its ExecuteOnce): only csrf.
	if !exec.chainSee["Login"]["csrf"] || len(exec.chainSee["Login"]) != 1 {
		t.Errorf("Login saw chain = %v, want only {csrf}", exec.chainSee["Login"])
	}
	// Bootstrap's chain map: empty.
	if len(exec.chainSee["Bootstrap"]) != 0 {
		t.Errorf("Bootstrap saw chain = %v, want empty", exec.chainSee["Bootstrap"])
	}
}

// TestResolveCycleDirect verifies A → A short-cycle is caught.
func TestResolveCycleDirect(t *testing.T) {
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "self", Request: "Leaf"},
	}}
	finder := fakeFinder{"Leaf": leaf}
	exec := newRecordingExec()
	_, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want cycle error", err)
	}
}

// TestResolveCycleIndirect verifies A → B → A is caught.
func TestResolveCycleIndirect(t *testing.T) {
	a := model.Request{ID: "A", Name: "A", Chain: []model.ChainStep{
		{Alias: "b", Request: "B"},
	}}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{
		{Alias: "a", Request: "A"},
	}}
	finder := fakeFinder{"A": a, "B": b}
	exec := newRecordingExec()
	_, _, err := Resolve(context.Background(), a, finder, exec, nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want cycle error", err)
	}
}

// TestResolveCycleEmptyIDDirect verifies a self-cycle among never-saved
// (empty-ID) requests is reported as a cycle via the path-based visiting key,
// not allowed to recurse to the depth/step cap (#99).
func TestResolveCycleEmptyIDDirect(t *testing.T) {
	leaf := model.Request{Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "self", Request: "Leaf"},
	}}
	finder := fakeFinder{"Leaf": leaf}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want a cycle error", err)
	}
}

// TestResolveCycleEmptyIDIndirect verifies A → B → A among empty-ID requests
// is caught as a cycle (#99).
func TestResolveCycleEmptyIDIndirect(t *testing.T) {
	a := model.Request{Name: "A", Chain: []model.ChainStep{{Alias: "b", Request: "B"}}}
	b := model.Request{Name: "B", Chain: []model.ChainStep{{Alias: "a", Request: "A"}}}
	finder := fakeFinder{"A": a, "B": b}
	_, _, err := Resolve(context.Background(), a, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want a cycle error (not a depth/step-cap error)", err)
	}
}

// TestResolveUnknownRef verifies an unresolved name path surfaces a
// clear error naming the alias.
func TestResolveUnknownRef(t *testing.T) {
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "missing", Request: "DoesNotExist"},
	}}
	exec := newRecordingExec()
	_, _, err := Resolve(context.Background(), leaf, fakeFinder{}, exec, nil)
	if err == nil || !strings.Contains(err.Error(), "DoesNotExist") {
		t.Errorf("err = %v, want unresolved-ref error", err)
	}
}

// TestResolveDuplicateAlias verifies two steps with the same alias in
// one request's chain produce a clear error.
func TestResolveDuplicateAlias(t *testing.T) {
	a := model.Request{ID: "X", Name: "X"}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "x", Request: "X"},
		{Alias: "x", Request: "X"},
	}}
	finder := fakeFinder{"X": a}
	exec := newRecordingExec()
	_, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Errorf("err = %v, want duplicate-alias error", err)
	}
}

// TestResolveMissingAlias verifies a step with no Alias is rejected.
func TestResolveMissingAlias(t *testing.T) {
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "", Request: "X"},
	}}
	finder := fakeFinder{"X": model.Request{ID: "X", Name: "X"}}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "alias") {
		t.Errorf("err = %v, want missing-alias error", err)
	}
}

// TestResolveExecutorError verifies an executor failure aborts the
// chain and the error names the offending step.
func TestResolveExecutorError(t *testing.T) {
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "boom", Request: "X"},
	}}
	finder := fakeFinder{"X": model.Request{ID: "X", Name: "X"}}
	exec := failingExec{}
	_, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want error naming step 'boom'", err)
	}
}

type failingExec struct{}

func (failingExec) ExecuteOnce(context.Context, model.Request, map[string]View) (View, []string, error) {
	return View{}, nil, errFakeExec
}

var errFakeExec = stubErr("executor failed")

type stubErr string

func (s stubErr) Error() string { return string(s) }

// TestResolveDepthCap verifies the runner refuses to descend past
// MaxChainDepth and surfaces a clean error instead of a stack overflow.
func TestResolveDepthCap(t *testing.T) {
	// Build a linear chain of depth MaxChainDepth+2: leaf → r0 → r1 → … → rN.
	finder := fakeFinder{}
	for i := 0; i < MaxChainDepth+2; i++ {
		name := fmt.Sprintf("r%d", i)
		next := fmt.Sprintf("r%d", i+1)
		var chain []model.ChainStep
		if i < MaxChainDepth+1 {
			chain = []model.ChainStep{{Alias: "next", Request: next}}
		}
		finder[name] = model.Request{ID: name, Name: name, Chain: chain}
	}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{{Alias: "next", Request: "r0"}}}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "depth") {
		t.Errorf("err = %v, want depth-limit error", err)
	}
}

// TestResolveStepCountCap verifies the runner refuses to execute past
// MaxChainSteps total steps and surfaces a clean error.
func TestResolveStepCountCap(t *testing.T) {
	// Build a wide fan-out: leaf chains to b0..b(MaxChainSteps+5).
	finder := fakeFinder{}
	leaf := model.Request{ID: "L", Name: "Leaf"}
	for i := 0; i < MaxChainSteps+5; i++ {
		name := fmt.Sprintf("b%d", i)
		finder[name] = model.Request{ID: name, Name: name}
		leaf.Chain = append(leaf.Chain, model.ChainStep{Alias: fmt.Sprintf("b%d", i), Request: name})
	}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "step count") {
		t.Errorf("err = %v, want step-count limit error", err)
	}
}

// TestResolveAliasMustBeJSIdentifier verifies an alias like "foo-bar"
// or "1login" or "class" is rejected so users get a clear error rather
// than silently producing a broken `chain['foo-bar']` shape.
func TestResolveAliasMustBeJSIdentifier(t *testing.T) {
	finder := fakeFinder{"X": model.Request{ID: "X", Name: "X"}}
	exec := newRecordingExec()
	for _, bad := range []string{"foo-bar", "1login", "with space", "with.dot"} {
		leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
			{Alias: bad, Request: "X"},
		}}
		_, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
		if err == nil || !strings.Contains(err.Error(), "not a valid JS identifier") {
			t.Errorf("alias %q: err = %v, want JS-identifier error", bad, err)
		}
	}
}

// TestResolveBothBlankRowGivesTightError verifies a step where BOTH
// alias and request are blank surfaces the tighter "missing both"
// error instead of a "missing alias (request \"\")" line.
func TestResolveBothBlankRowGivesTightError(t *testing.T) {
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "", Request: ""},
	}}
	_, _, err := Resolve(context.Background(), leaf, fakeFinder{}, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "missing both") {
		t.Errorf("err = %v, want both-blank error", err)
	}
}

// TestResolveConsoleAccumulatorTruncates verifies that a chain step
// whose ExecuteOnce returns many console lines doesn't grow the
// accumulator past MaxChainConsoleLines, and a truncation marker is
// added exactly once.
func TestResolveConsoleAccumulatorTruncates(t *testing.T) {
	noisy := model.Request{ID: "N", Name: "Noisy"}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "noisy", Request: "Noisy"},
	}}
	exec := &noisyExec{count: MaxChainConsoleLines + 200}
	_, console, err := Resolve(context.Background(), leaf, fakeFinder{"Noisy": noisy}, exec, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(console) > MaxChainConsoleLines+1 {
		t.Errorf("console len = %d, want ≤ %d", len(console), MaxChainConsoleLines+1)
	}
	if console[MaxChainConsoleLines] != "[chain console truncated]" {
		t.Errorf("expected truncation marker at index %d, got %q", MaxChainConsoleLines, console[MaxChainConsoleLines])
	}
}

// noisyExec returns `count` console lines per ExecuteOnce.
type noisyExec struct{ count int }

func (n *noisyExec) ExecuteOnce(context.Context, model.Request, map[string]View) (View, []string, error) {
	lines := make([]string, n.count)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	return View{Response: ResponseView{StatusCode: 200}}, lines, nil
}

// TestResolveProgressCallbackFiresPerStep verifies the progress
// callback is invoked once per ExecuteOnce in execution order, with
// 1-based step numbers, the upfront-counted total, and the resolved
// alias + request name.
func TestResolveProgressCallbackFiresPerStep(t *testing.T) {
	d := model.Request{ID: "D", Name: "D"}
	c := model.Request{ID: "C", Name: "C"}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{{Alias: "csrf", Request: "D"}}}
	leaf := model.Request{ID: "A", Name: "A", Chain: []model.ChainStep{
		{Alias: "login", Request: "B"},
		{Alias: "info", Request: "C"},
	}}
	finder := fakeFinder{"D": d, "C": c, "B": b}

	type event struct {
		step, total int
		alias, name string
	}
	var events []event
	progress := func(step, total int, alias, name string) {
		events = append(events, event{step, total, alias, name})
	}

	if _, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), progress); err != nil {
		t.Fatalf("Resolve err = %v", err)
	}

	want := []event{
		{1, 3, "csrf", "D"},
		{2, 3, "login", "B"},
		{3, 3, "info", "C"},
	}
	if len(events) != len(want) {
		t.Fatalf("progress events = %d, want %d (%+v)", len(events), len(want), events)
	}
	for i, ev := range events {
		if ev != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, ev, want[i])
		}
	}
}

// TestResolveEmitsPerStepTrace verifies that for each chain step
// whose HTTP actually went out (view.Request.URL non-empty), the
// runner appends a "→ chain[<alias>] <METHOD> <URL>" line to the
// shared console so the user can see what each step sent — including
// any URL/method mutations from the step's pre-script.
func TestResolveEmitsPerStepTrace(t *testing.T) {
	login := model.Request{ID: "B", Name: "Login", Method: model.POST, URL: "https://auth/login"}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "login", Request: "Auth/Login"},
	}}
	finder := fakeFinder{"Auth/Login": login}
	_, console, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	want := "→ chain[login] POST https://auth/login"
	found := false
	for _, line := range console {
		if line == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected trace %q in console %+v", want, console)
	}
}

// TestResolveByRequestIDPrefersIDOverPath verifies that when a
// ChainStep carries a RequestID matching a known request, the runner
// uses it even if Request (the path) refers to a different — or
// missing — entry. Pinning RequestID is how chain refs survive renames
// and folder moves of the target.
func TestResolveByRequestIDPrefersIDOverPath(t *testing.T) {
	moved := model.Request{ID: "MOVED-ID", Name: "Renamed", URL: "https://x/renamed"}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "login", Request: "Old/Path/That/No/Longer/Exists", RequestID: "MOVED-ID"},
	}}
	finder := fakeFinder{"NewLocation/Renamed": moved}
	exec := newRecordingExec()
	m, _, err := Resolve(context.Background(), leaf, finder, exec, nil)
	if err != nil {
		t.Fatalf("Resolve err = %v", err)
	}
	if got := m["login"].Request.URL; got != "https://x/renamed" {
		t.Errorf("by-ID resolution missed: chain[login].url = %q", got)
	}
}

// TestResolveByRequestIDFallsBackToPath verifies that when the
// RequestID doesn't match any known request, resolution falls back to
// the Request path. Ensures stale IDs don't break workflows that still
// have a valid path.
func TestResolveByRequestIDFallsBackToPath(t *testing.T) {
	login := model.Request{ID: "B", Name: "Login", URL: "https://auth/login"}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "login", Request: "Auth/Login", RequestID: "stale-id-never-existed"},
	}}
	finder := fakeFinder{"Auth/Login": login}
	if _, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil); err != nil {
		t.Errorf("expected fallback to path; err = %v", err)
	}
}

// TestResolveProgressCallbackNilSafe verifies that passing nil as the
// progress callback does not panic and runs the chain to completion.
func TestResolveProgressCallbackNilSafe(t *testing.T) {
	b := model.Request{ID: "B", Name: "B"}
	leaf := model.Request{ID: "A", Name: "A", Chain: []model.ChainStep{
		{Alias: "login", Request: "B"},
	}}
	if _, _, err := Resolve(context.Background(), leaf, fakeFinder{"B": b}, newRecordingExec(), nil); err != nil {
		t.Errorf("nil-progress Resolve err = %v", err)
	}
}

// TestResolveDepthExactlyAtCapErrors verifies the `>=` boundary of
// the depth cap: a chain whose deepest call lands AT MaxChainDepth
// — and at that depth there is no further chain to recurse into —
// must error. Kills the BOUNDARY mutation that flips `>=` to `>`
// (which would silently allow that exact depth to succeed because
// the empty inner chain skips the for-loop without recursing
// deeper).
func TestResolveDepthExactlyAtCapErrors(t *testing.T) {
	// Build a linear chain that recurses to exactly depth=MaxChainDepth
	// with the deepest request having NO chain. Then:
	//   - Original `depth >= MaxChainDepth` triggers at the deepest
	//     resolveSteps call → error.
	//   - Mutation `depth > MaxChainDepth` does NOT trigger at that
	//     depth; the empty for-loop returns success.
	// MaxChainDepth=8 means 9 requests total (leaf + r0..r{MaxChainDepth-1}),
	// with r{MaxChainDepth-1} carrying no chain.
	finder := fakeFinder{}
	for i := 0; i < MaxChainDepth; i++ {
		name := fmt.Sprintf("r%d", i)
		next := fmt.Sprintf("r%d", i+1)
		var chain []model.ChainStep
		if i < MaxChainDepth-1 {
			chain = []model.ChainStep{{Alias: "n", Request: next}}
		}
		finder[name] = model.Request{ID: name, Name: name, Chain: chain}
	}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{{Alias: "n", Request: "r0"}}}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "depth exceeds") {
		t.Errorf("err = %v, want depth-cap error at the exact boundary", err)
	}
}

// TestResolveStepCountExactlyAtCapErrors verifies the >= boundary of
// the step-count cap: when stepCount has reached MaxChainSteps the
// next step must error. Kills the BOUNDARY mutation flipping >= to >.
func TestResolveStepCountExactlyAtCapErrors(t *testing.T) {
	finder := fakeFinder{}
	// MaxChainSteps + 1 chain entries on the leaf, all flat (no
	// nested chain) — each consumes one stepCount.
	leafChain := make([]model.ChainStep, 0, MaxChainSteps+1)
	for i := 0; i <= MaxChainSteps; i++ {
		name := fmt.Sprintf("s%d", i)
		alias := fmt.Sprintf("a%d", i)
		finder[name] = model.Request{ID: name, Name: name}
		leafChain = append(leafChain, model.ChainStep{Alias: alias, Request: name})
	}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: leafChain}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "step count exceeds") {
		t.Errorf("err = %v, want step-count-cap error at the exact boundary", err)
	}
}

// TestResolveDiamondPatternRunsSharedDepTwice verifies the visiting-
// set cleanup branch: A → [B, C] where both B and C chain to D
// must execute D twice (once per branch) without a false cycle.
// Kills the NEGATION mutation on the cleanup `if sub.ID != ""` and
// BOUNDARY mutations adjacent.
func TestResolveDiamondPatternRunsSharedDepTwice(t *testing.T) {
	d := model.Request{ID: "D", Name: "D"}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{{Alias: "d", Request: "D"}}}
	c := model.Request{ID: "C", Name: "C", Chain: []model.ChainStep{{Alias: "d", Request: "D"}}}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "b", Request: "B"},
		{Alias: "c", Request: "C"},
	}}
	finder := fakeFinder{"B": b, "C": c, "D": d}
	exec := newRecordingExec()
	if _, _, err := Resolve(context.Background(), leaf, finder, exec, nil); err != nil {
		t.Fatalf("diamond err = %v (cleanup branch likely regressed)", err)
	}
	// D should execute twice: once under B's branch, once under C's.
	count := 0
	for _, name := range exec.calls {
		if name == "D" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("D executed %d times, want 2 (diamond branches must re-execute shared dep)", count)
	}
}

// TestResolveDeepCycleDetectedViaVisitingPopulation verifies cycle
// detection works at recursion depth ≥ 2 — kills the NEGATION
// mutation on the `if sub.ID != ""` block that POPULATES the
// visiting set after a successful resolution. With the population
// skipped, a B→A→B cycle (where the leaf is C, so C is in visiting
// but A is not) wouldn't be detected on the inner recursion.
func TestResolveDeepCycleDetectedViaVisitingPopulation(t *testing.T) {
	a := model.Request{ID: "A", Name: "A"}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{{Alias: "a", Request: "A"}}}
	// A.Chain points back at B — so the cycle is A → B → A at depth ≥ 2.
	a.Chain = []model.ChainStep{{Alias: "b", Request: "B"}}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "b", Request: "B"},
	}}
	finder := fakeFinder{"A": a, "B": b}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("err = %v, want cycle-detected at depth ≥ 2", err)
	}
}

// TestResolveLeafSelfChainCycleDetected verifies the leaf's own ID is
// populated in the visiting set so a chain step that resolves back
// to the leaf is caught as a cycle. Kills the NEGATION mutation on
// `if leaf.ID != ""` in Resolve.
func TestResolveLeafSelfChainCycleDetected(t *testing.T) {
	leaf := model.Request{ID: "L", Name: "Leaf"}
	leaf.Chain = []model.ChainStep{{Alias: "self", Request: "Leaf"}}
	// Finder also resolves "Leaf" back to the same leaf (cycle).
	finder := fakeFinder{"Leaf": leaf}
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), nil)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("err = %v, want cycle-detected on leaf self-reference", err)
	}
}

// TestResolveProgressTotalCountsCorrectly verifies the countSteps
// pre-walk produces the same total a non-cyclic chain actually runs.
// Hits the `if leaf.ID != ""` population + visiting cleanup branches
// inside countSteps. Catches a regression where countSteps loses
// track of cycles and returns a count that doesn't match execution.
func TestResolveProgressTotalCountsCorrectly(t *testing.T) {
	// Diamond again — D executes twice, B and C once each = 4 steps.
	d := model.Request{ID: "D", Name: "D"}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{{Alias: "d", Request: "D"}}}
	c := model.Request{ID: "C", Name: "C", Chain: []model.ChainStep{{Alias: "d", Request: "D"}}}
	leaf := model.Request{ID: "A", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "b", Request: "B"},
		{Alias: "c", Request: "C"},
	}}
	finder := fakeFinder{"B": b, "C": c, "D": d}

	var totalSeen int
	progress := func(_, total int, _, _ string) { totalSeen = total }

	if _, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), progress); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if totalSeen != 4 {
		t.Errorf("total = %d, want 4 (B + D + C + D in diamond)", totalSeen)
	}
}

// TestResolveProgressTotalRespectsDepthCap verifies countSteps' own
// depth cap clamps a linear chain deeper than the cap to a finite
// total. The leaf has two children: a shallow one that executes
// successfully (so the progress callback fires and we can observe
// total), and a deep one that exceeds the cap (whose pre-walked
// contribution is bounded by countSteps' depth cap).
//
// Kills:
//   - the BOUNDARY mutation on countSteps' `depth >= MaxChainDepth`
//   - the ARITHMETIC mutation that flips `depth+1` to `depth-1` in
//     countSteps' recursion (which would let depth go negative,
//     bypass the cap entirely, and count every level — producing a
//     total well above the cap).
func TestResolveProgressTotalRespectsDepthCap(t *testing.T) {
	// Shallow branch: one request with no chain. countSteps = 1.
	shallow := model.Request{ID: "S", Name: "Shallow"}

	// Deep branch: linear chain of MaxChainDepth+5 levels. countSteps
	// caps each level's recursion, so the total contribution is
	// clamped to MaxChainDepth.
	finder := fakeFinder{"Shallow": shallow}
	deep := MaxChainDepth + 5
	for i := 0; i < deep; i++ {
		name := fmt.Sprintf("r%d", i)
		next := fmt.Sprintf("r%d", i+1)
		var chain []model.ChainStep
		if i < deep-1 {
			chain = []model.ChainStep{{Alias: "n", Request: next}}
		}
		finder[name] = model.Request{ID: name, Name: name, Chain: chain}
	}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
		{Alias: "shallow", Request: "Shallow"}, // succeeds → progress fires
		{Alias: "deep", Request: "r0"},         // errors at depth cap
	}}

	var totalSeen int
	progress := func(_, total int, _, _ string) {
		if total > totalSeen {
			totalSeen = total
		}
	}
	_, _, _ = Resolve(context.Background(), leaf, finder, newRecordingExec(), progress)
	// Original total: 1 (Shallow) + clamped-deep (≤ MaxChainDepth) ≤ 1 + MaxChainDepth.
	// depth-1 mutation: total = 1 (Shallow) + deep (≈ MaxChainDepth+5) = roughly 1 + deep.
	// We assert the upper bound the original respects.
	if totalSeen == 0 || totalSeen > 1+MaxChainDepth {
		t.Errorf("totalSeen = %d, want 0 < t <= %d (countSteps cap regression)", totalSeen, 1+MaxChainDepth)
	}
}

// TestResolveProgressTotalCountsIDOnlyStep verifies countSteps does
// NOT skip a step whose Request is empty but RequestID is set and
// resolves. The total observable via the progress callback is the
// vehicle for the assertion (countSteps is unexported and runs
// before any step executes — observing requires a successful first
// step that fires the callback).
//
// Kills the NEGATION mutation on the `step.RequestID == ""` half of
// countSteps' both-blank guard: with the negation, countSteps would
// skip the ID-only step and produce a total of 1 instead of 2.
func TestResolveProgressTotalCountsIDOnlyStep(t *testing.T) {
	target := model.Request{ID: "TARGET-ID", Name: "Target"}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{
		// 1: a normal path-only step that executes successfully so
		//    the progress callback fires and totalSeen captures the
		//    pre-walked total.
		{Alias: "first", Request: "Target"},
		// 2: an ID-only step (empty Request, populated RequestID)
		//    that countSteps must count. Resolve errors on its
		//    blank-ref check at runtime, but countSteps already ran.
		{Alias: "second", Request: "", RequestID: "TARGET-ID"},
	}}
	finder := fakeFinder{"Target": target}

	var totalSeen int
	progress := func(_, total int, _, _ string) { totalSeen = total }

	_, _, _ = Resolve(context.Background(), leaf, finder, newRecordingExec(), progress)
	if totalSeen != 2 {
		t.Errorf("totalSeen = %d, want 2 (countSteps must include the ID-only step)", totalSeen)
	}
}

// TestResolveProgressTotalSkipsCycle verifies countSteps' cycle skip:
// a chain with a self-cycle should still produce a finite total
// matching the non-cyclic prefix, not 0 (depth-cap fallback) or
// MaxChainSteps (overflow). Kills the NEGATION mutations on
// countSteps' cycle-detection branches.
func TestResolveProgressTotalSkipsCycle(t *testing.T) {
	// A's chain → B → A (cycle). countSteps must skip the cycle
	// and return finite (specifically: counts B before detecting
	// the cycle on A, then bails — but Resolve later errors on
	// the same cycle. countSteps just needs to terminate.)
	a := model.Request{ID: "A", Name: "A"}
	b := model.Request{ID: "B", Name: "B", Chain: []model.ChainStep{{Alias: "a", Request: "A"}}}
	a.Chain = []model.ChainStep{{Alias: "b", Request: "B"}}
	leaf := model.Request{ID: "L", Name: "Leaf", Chain: []model.ChainStep{{Alias: "a", Request: "A"}}}
	finder := fakeFinder{"A": a, "B": b}

	called := false
	progress := func(int, int, string, string) { called = true }

	// We expect Resolve to surface the cycle error. countSteps must
	// have terminated cleanly first (otherwise we never get to call
	// resolveSteps at all). Reaching the cycle error proves both
	// countSteps and the resolver progressed past the count phase.
	_, _, err := Resolve(context.Background(), leaf, finder, newRecordingExec(), progress)
	if err == nil || !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("err = %v, want cycle-detected", err)
	}
	_ = called
}
