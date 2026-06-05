# vars

Substitutes `{{variable}}` templates in URLs, headers, query params, and request bodies. The package is intentionally tiny — it is the one place that knows the syntax and precedence rules for Helena's variable references.

A `Resolver` is built from one or more named scopes ordered lowest precedence first; later scopes override earlier ones. Typical call site: `vars.New(collectionVars, envVars, dotEnvVars)`. Substitution iterates to a fixed point so that variables referring to other variables (e.g. `{{url}}` -> `{{proto}}://{{host}}` -> `https://api.example.com`) resolve in a single `Resolve` call, while a hard pass cap stops cyclic references from looping forever.

## Public API

### Types
- `Resolver` — substitution engine over a stack of scope maps.

### Functions / methods
- `New(scopes ...map[string]string) *Resolver` — builds a resolver; scopes are low-to-high precedence.
- `(*Resolver).WithFallback(fn func(name string) (string, bool)) *Resolver` — attaches a dynamic lookup consulted for any name no scope resolves, so callers can inject namespaced values (e.g. `{{chain.<alias>.response.json.token}}` backed by `chain.VarLookup`) without the resolver knowing their source. Returns the resolver for chaining; `nil` clears it.
- `(*Resolver).Lookup(name string) (string, bool)` — returns the highest-precedence value for `name` (scopes first, then the fallback).
- `(*Resolver).Resolve(s string) (string, []string)` — substitutes every `{{name}}` to a fixed point and returns the result plus the names that remain unresolved (deduped, first-seen order).

## Dependencies

### Internal
None. Callers (`httpclient`, `session`, `exporter`) build scope maps and pass them in.

### External (standard library only)
- `regexp` — pattern for `{{ name }}` matching.
- `strings` — whitespace trimming inside captures.
