# vars

Substitutes `{{variable}}` templates in URLs, headers, query params, and request bodies. The package is intentionally tiny — it is the one place that knows the syntax and precedence rules for Helena's variable references.

A `Resolver` is built from one or more named scopes ordered lowest precedence first; later scopes override earlier ones. Typical call site: `vars.New(dotEnvVars, collectionVars, envVars)` (lowest precedence first). **Scope** (user-authored) variables expand **recursively**, so a variable may compose others (e.g. `{{url}}` -> `{{proto}}://{{host}}` -> `https://api.example.com`) in a single `Resolve` call; an acyclic chain resolves fully at any depth, and a cyclic reference (`a → b → a`) is detected and reported as unresolved rather than looping. **Fallback** (dynamic) values — e.g. `{{chain.<alias>.…}}` backed by a response body — are substituted **verbatim and never re-expanded**: this is a deliberate security boundary so server- or chain-controlled data cannot smuggle a `{{secret}}` reference that would expand against the user's scopes.

## Public API

### Types
- `Resolver` — substitution engine over a stack of scope maps.

### Functions / methods
- `New(scopes ...map[string]string) *Resolver` — builds a resolver; scopes are low-to-high precedence.
- `(*Resolver).WithFallback(fn func(name string) (string, bool)) *Resolver` — attaches a dynamic lookup consulted for any name no scope resolves, so callers can inject namespaced values (e.g. `{{chain.<alias>.response.json.token}}` backed by `chain.VarLookup`) without the resolver knowing their source. Returns the resolver for chaining; `nil` clears it.
- `(*Resolver).Lookup(name string) (string, bool)` — returns the highest-precedence value for `name` (scopes first, then the fallback).
- `(*Resolver).Resolve(s string) (string, []string)` — substitutes every `{{name}}`, expanding scope values recursively and freezing fallback values, and returns the result plus the names that are unresolvable or cyclic (deduped, first-seen order).
- `Dynamic(name string) (string, bool)` — a fallback that resolves Postman-style dynamic ("magic") variables (`{{$guid}}`, `{{$randomUUID}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$randomInt}}`, `{{$randomFloat}}`, `{{$randomBoolean}}`, `{{$randomFirstName}}`/`LastName`/`FullName`/`Email`, `{{$randomColor}}`). Only `$`-prefixed names are claimed; an unknown `$name` returns `("", false)` so it is still reported missing. Each call generates a fresh value (and, being a fallback, is frozen — never re-expanded).
- `Compose(lookups ...func(string) (string, bool)) func(string) (string, bool)` — combines several fallbacks into one, returning the first match (nil lookups skipped). Used to attach `Dynamic` alongside `chain.VarLookup`.
- `PromptVars(texts ...string) []string` — finds `{{?Name}}` prompt-variable references (#86) and returns the distinct prompt keys (each the captured name *including* its `?` marker), first-seen order. The UI collects a value per key at Send time and injects a scope under that key; because the key carries the `?`, it only ever matches `{{?...}}` references and never collides with a normal `{{Name}}`.
- `PromptLabel(key string) string` — strips the `?` marker from a prompt key for a human-facing prompt label.

## Dependencies

### Internal
None. Callers (`httpclient`, `session`, `exporter`) build scope maps and pass them in.

### External (standard library only)
- `regexp` — pattern for `{{ name }}` matching.
- `strings` — whitespace trimming inside captures.
