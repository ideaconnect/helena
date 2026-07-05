package vars

import (
	"regexp"
	"strings"
)

// varRe matches a {{ name }} template, capturing the (untrimmed) name.
var varRe = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)

// Resolver substitutes {{name}} templates using a set of named scopes.
// Scopes are supplied lowest-precedence first; later scopes override earlier
// ones — e.g. New(collectionVars, envVars, dotEnvVars).
type Resolver struct {
	scopes   []map[string]string
	fallback func(name string) (string, bool)
}

// New builds a Resolver from scopes ordered low to high precedence.
func New(scopes ...map[string]string) *Resolver {
	return &Resolver{scopes: scopes}
}

// WithFallback attaches a dynamic lookup consulted for any name no scope
// resolves. It lets callers inject namespaced values — e.g.
// {{chain.<alias>.response.json.token}} backed by chain results — without the
// resolver knowing their source. Returns r for chaining. Passing nil clears it.
func (r *Resolver) WithFallback(fn func(name string) (string, bool)) *Resolver {
	r.fallback = fn
	return r
}

// lookupScope returns the highest-precedence SCOPE value for name, without
// consulting the dynamic fallback.
func (r *Resolver) lookupScope(name string) (string, bool) {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if v, ok := r.scopes[i][name]; ok {
			return v, true
		}
	}
	return "", false
}

// Lookup returns the highest-precedence value for name, falling back to the
// dynamic lookup (if any) when no scope has it.
func (r *Resolver) Lookup(name string) (string, bool) {
	if v, ok := r.lookupScope(name); ok {
		return v, true
	}
	if r.fallback != nil {
		return r.fallback(name)
	}
	return "", false
}

// Resolve substitutes every {{name}} in s with its value.
//
// Scope values (user-authored collection / environment / .env variables) are
// expanded RECURSIVELY, so a variable may compose others within a single call
// (e.g. {{url}} → "{{proto}}://{{host}}" → "https://x.com"). A cyclic scope
// reference (a → b → a) is detected, left intact, and reported as unresolved
// rather than looping; an arbitrarily deep but acyclic chain resolves fully.
//
// Fallback values (dynamic — e.g. {{chain.<alias>.…}} backed by a response
// body) are substituted VERBATIM and are NEVER re-scanned for templates. This
// is a security boundary: server- or chain-controlled data therefore cannot
// smuggle a {{secret}} reference that would re-expand against the user's
// scopes. Composition is a property of trusted, user-authored scopes only.
//
// It returns the result and the names that are unresolvable (no scope and no
// fallback) or cyclic, deduped in first-seen order.
func (r *Resolver) Resolve(s string) (string, []string) {
	st := &resolveState{
		visiting: map[string]bool{},
		missing:  map[string]bool{},
		memo:     map[string]string{},
		noCache:  map[string]bool{},
	}
	out := r.expand(s, st)
	return out, st.order
}

// resolveState is the mutable bookkeeping for one Resolve call. Keeping it in a
// struct (rather than five threaded parameters) keeps expand readable and makes
// the memo/noCache pair — the fix for exponential re-expansion — cheap to carry.
type resolveState struct {
	visiting map[string]bool   // scope vars currently being expanded (cycle guard)
	missing  map[string]bool   // unresolvable/cyclic names already reported
	memo     map[string]string // fully-expanded value of each cache-safe scope var
	noCache  map[string]bool   // scope vars whose expansion touched a cycle (unsafe to memo)
	order    []string          // missing names in first-seen order
}

// expand replaces the top-level {{name}} templates in s. A scope value is
// expanded recursively (visiting guards against cycles, so depth is bounded by
// the number of distinct variables); a fallback value is frozen. Unresolvable
// and cyclic names accumulate into st.order via st.missing.
//
// Each scope variable's fully-expanded value is memoized within the Resolve, so
// a value referenced N times (e.g. "{{a}}{{a}}") expands once, not N times —
// without this a branching acyclic scope graph (v0="{{v1}}{{v1}}", …) costs
// O(2^depth) and can wedge the caller. A variable whose expansion truncated a
// cycle is context-dependent (its result would differ if reused off the cycle),
// so every var on the stack at cycle-detection is marked no-cache; a cached
// value is therefore always cycle-free and independent of the visiting stack.
func (r *Resolver) expand(s string, st *resolveState) string {
	return varRe.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimSpace(varRe.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}
		if v, ok := r.lookupScope(name); ok {
			if cached, ok := st.memo[name]; ok {
				return cached
			}
			if st.visiting[name] {
				// Cyclic scope reference: stop, keep the template, report it, and
				// poison the cache for every var currently on the stack.
				for onStack := range st.visiting {
					st.noCache[onStack] = true
				}
				markMissing(name, st.missing, &st.order)
				return match
			}
			st.visiting[name] = true
			out := r.expand(v, st)
			delete(st.visiting, name)
			if !st.noCache[name] {
				st.memo[name] = out
			}
			return out
		}
		if r.fallback != nil {
			if v, ok := r.fallback(name); ok {
				return v // frozen: untrusted dynamic values are not re-expanded
			}
		}
		markMissing(name, st.missing, &st.order)
		return match
	})
}

// markMissing records name once, preserving first-seen order.
func markMissing(name string, missing map[string]bool, order *[]string) {
	if !missing[name] {
		missing[name] = true
		*order = append(*order, name)
	}
}
