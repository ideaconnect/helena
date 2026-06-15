# vars — Workflow

## Resolving a `{{var}}` reference in a request
1. The session collects enabled variables from the active environment (and any other applicable scope) into `map[string]string` values.
2. Caller invokes `vars.New(scope1, scope2, ...)` with scopes ordered lowest- to highest-precedence.
3. Caller calls `r.Resolve("https://{{host}}/users/{{id}}")`.
4. `expand` runs the regex over the string; each match resolves its name. A
   **scope** hit (user-authored variable) is expanded recursively so it may
   compose other variables; a **fallback** hit (dynamic, e.g. a chain result)
   is substituted **verbatim** and never re-scanned.
5. `Resolve` returns the substituted string and the list of names that are
   unresolvable (no scope, no fallback) or cyclic.

## Chained (composed) scope-variable expansion
1. `Resolve("{{url}}/health")` with `url -> "{{proto}}://{{host}}"`, `proto -> "https"`, `host -> "example.com"`.
2. `{{url}}` resolves to its scope value `{{proto}}://{{host}}`, which is then
   expanded recursively in place.
3. `{{proto}}` and `{{host}}` resolve to `https` and `example.com`, yielding
   `https://example.com/health` with no unresolved names.
4. Composition is unbounded in depth (a 15-deep acyclic chain resolves fully);
   recursion depth is bounded by the number of distinct variables, since the
   `visiting` set blocks re-entering a name already on the stack.

## Frozen fallback values (injection boundary)
1. `Resolve("{{chain.login.response.body}}")` where the fallback returns a
   server-controlled body like `leak={{secret}}`.
2. The fallback value is substituted **verbatim**: the result is
   `leak={{secret}}`, NOT `leak=<the user's secret>`.
3. Untrusted dynamic data therefore cannot smuggle a `{{secret}}` reference
   that would re-expand against the user's scopes. Composition is a property of
   trusted, user-authored scopes only.

## Cycle and missing-name handling
1. With `a -> "{{b}}"` and `b -> "{{a}}"`, expanding `{{a}}` pushes `a` then `b`
   onto the `visiting` set; re-encountering `a` is detected as a cycle.
2. The cyclic name is left as a literal `{{a}}` and reported as unresolved.
3. A name with no scope and no fallback is likewise reported, each once in
   first-seen order.
4. Caller (UI) shows those names as unresolved without crashing or hanging.

## Scope precedence at lookup
1. `New(low, high)` stores `[low, high]` in `scopes`.
2. `Lookup("x")` iterates `i = 1, 0` so it tries `high` first.
3. The first map containing `x` wins; nothing else is consulted.
