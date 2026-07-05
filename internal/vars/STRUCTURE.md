# vars — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level godoc only. |
| [vars.go](vars.go) | The `Resolver` type, regex, recursive `expand`, and missing-name collector. |
| [dynamic.go](dynamic.go) | `Dynamic` — Postman-style `{{$guid}}`/`{{$timestamp}}`/`{{$randomInt}}`/… magic-variable fallback; `Compose` (combine several fallbacks); `uuidV4` and the curated faker tables. |
| [prompt.go](prompt.go) | `PromptVars` / `PromptLabel` — extract `{{?Name}}` ask-at-Send-time prompt-variable keys (#86) from template text. |
| [prompt_test.go](prompt_test.go) | Prompt-key extraction (distinct, first-seen, `?`-marker), label stripping, and end-to-end resolution via an injected scope. |
| [dynamic_test.go](dynamic_test.go) | Format/range/freshness tests for each generator, `Compose` ordering, and the resolver-with-Dynamic-fallback integration (unknown `$name` still reported missing). |
| [vars_test.go](vars_test.go) | Tests for plain substitution, whitespace tolerance, precedence stacking, chained refs, missing-name reporting, cycle termination, frozen-fallback injection, deep-chain resolution, and empty templates. |
| [fallback_test.go](fallback_test.go) | `WithFallback`: the dynamic lookup resolves names no scope has, scopes still win, and a nil fallback is unchanged behavior. |

## Type catalog

### `Resolver` — [vars.go:17](vars.go#L17)
A wrapper over an ordered slice of scope maps.
- `scopes` (unexported) — passed to `New` low-to-high; `Lookup` scans from the top of the stack down, so the last scope wins.
- `fallback` (unexported) — optional `func(name) (string, bool)` set by `WithFallback`, consulted by `Lookup`/`expand` only when no scope resolves a name. Used to plug in the `{{chain.<alias>…}}` namespace (`chain.VarLookup`) without coupling `vars` to `chain`. **A fallback value is substituted verbatim and never re-scanned** — the injection boundary (see `expand`).

## Non-trivial internals

### `varRe` — [vars.go:9](vars.go#L9)
Regex `\{\{\s*([^{}]*?)\s*\}\}` — matches `{{name}}` allowing inner whitespace, captures the name without `{}` characters so nested templates can't confuse the matcher.

### `lookupScope` / `Lookup`
`lookupScope` scans the scope stack top-down (last scope wins) and is the scope-only resolver. `Lookup` is `lookupScope` plus the fallback — used by external callers and tests.

### `expand` (recursive substitution)
The core of `Resolve`. For each `{{name}}`: a **scope** value is expanded recursively (so user-authored variables compose), guarded by a `visiting` set that detects cycles (`a → b → a`) and bounds recursion depth to the number of distinct variables; a **fallback** value is returned **frozen** (verbatim, not re-scanned), which is the security boundary preventing server/chain data from re-expanding against the user's scopes. Unknown names, cyclic names, and empty `{{}}` are passed through; the first two are recorded via `markMissing`. Per-Resolve bookkeeping lives in `resolveState` (visiting / missing / memo / noCache / order). Each scope variable's fully-expanded value is **memoized** so a value referenced N times (e.g. `"{{a}}{{a}}"`) expands once, not N — without this a branching acyclic scope graph costs `O(2^depth)` and can wedge the caller. A variable whose expansion truncated a cycle is context-dependent, so every var on the stack at cycle detection is marked `noCache`; a memoized value is therefore always cycle-free and independent of the visiting stack.

### `markMissing`
Records an unresolvable or cyclic name once, preserving first-seen order — the UI uses the returned slice to surface "unresolved" badges.
