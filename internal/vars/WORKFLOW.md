# vars — Workflow

## Resolving a `{{var}}` reference in a request
1. The session collects enabled variables from the active environment (and any other applicable scope) into `map[string]string` values.
2. Caller invokes `vars.New(scope1, scope2, ...)` with scopes ordered lowest- to highest-precedence.
3. Caller calls `r.Resolve("https://{{host}}/users/{{id}}")`.
4. `substituteOnce` runs the regex over the string; each match calls `Lookup`, which walks scopes top-to-bottom and returns the first hit.
5. The fixed-point loop reruns `substituteOnce` until the string stops changing or `maxPasses` is reached.
6. `Resolve` returns the substituted string and the list of names that still appear unresolved.

## Chained variable expansion
1. `Resolve("{{url}}/health")` with `url -> "{{proto}}://{{host}}"`, `proto -> "https"`, `host -> "example.com"`.
2. Pass 1 expands `{{url}}` to `{{proto}}://{{host}}/health`.
3. Pass 2 expands `{{proto}}` and `{{host}}` to `https://example.com/health`.
4. Pass 3 sees no change, so the loop exits at the fixed point with no unresolved names.

## Cycle and missing-name handling
1. With `a -> "{{b}}"` and `b -> "{{a}}"`, each pass swaps `{{a}}` and `{{b}}` but never makes progress beyond that.
2. The loop hits `maxPasses` and exits.
3. `unresolvedNames` scans the final string for any remaining `{{name}}` and returns each once.
4. Caller (UI) shows those names as unresolved without crashing or hanging.

## Scope precedence at lookup
1. `New(low, high)` stores `[low, high]` in `scopes`.
2. `Lookup("x")` iterates `i = 1, 0` so it tries `high` first.
3. The first map containing `x` wins; nothing else is consulted.
