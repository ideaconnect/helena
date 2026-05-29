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
