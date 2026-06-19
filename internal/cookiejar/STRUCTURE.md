# cookiejar — Structure

## Files

| File | Responsibility |
| --- | --- |
| [cookiejar.go](cookiejar.go) | Everything: the `Jar` and `Cookie` types, the `http.CookieJar` methods (`SetCookies`/`Cookies`), the management API (`All`/`Len`/`Set`/`Remove`/`Clear`), and the RFC-6265 matching helpers. The package godoc lives at the top of this file. |
| [cookiejar_test.go](cookiejar_test.go) | Set/send, host-only vs domain scoping, foreign/super-cookie rejection, Secure gating, path scoping + ordering, Max-Age/Expires + purge, slot-preserving updates, manual Set/Remove/Clear, `All` sort stability, input guards, the matching-helper unit tests, and a `-race` concurrency hammer. |

## Type catalog

### `Cookie` — [cookiejar.go](cookiejar.go)
Public, viewer-facing cookie: `Name`, `Value`, `Domain` (canonical, no leading
dot), `Path`, `Expires` (zero ⇒ session cookie), `Secure`, `HTTPOnly`,
`HostOnly` (set without a `Domain` attribute ⇒ exact-host match only).

### `entry` — [cookiejar.go](cookiejar.go)
Internal: embeds `Cookie` plus `created` (kept stable across updates per RFC
6265) and a monotonic `seq` used as the stable tiebreaker in send/display order.

### `Jar` — [cookiejar.go](cookiejar.go)
- `mu sync.RWMutex` — guards every accessor (sends run on a worker goroutine; the
  viewer reads on the UI goroutine).
- `m map[string]entry` — keyed by `key(domain, path, name)` so a re-`Set` of the
  same triple updates rather than duplicates.
- `seq int64` — increments on each new slot, for ordering tiebreaks.
- `now func() time.Time` — the clock; swapped in tests to drive expiry.

## Non-trivial internals

### `key(domain, path, name)`
NUL-joined composite key implementing RFC 6265's "a cookie is identified by
domain+path+name" rule.

### `store` / `liveEntries`
`store` upserts at a key, copying `created`/`seq` from any prior entry so updates
don't reorder. `liveEntries` is the shared filtered-iteration helper used by
`Cookies` and `All`: it deletes expired entries as it walks and returns those
matching an optional predicate. Both run under the caller's lock.

### `domainAttr`
Resolves a `Set-Cookie` `Domain` against the response host: empty ⇒ host-only;
otherwise canonicalises, requires the host to be the domain or a sub-domain of
it, and rejects a dotless super-cookie domain (`com`) used as a suffix. An
IP-literal host (detected via `net.ParseIP`) is special-cased: a `Domain`
attribute is accepted only when it equals the host and the cookie is forced
host-only, so a crafted dotted suffix can't leak across sibling IPs.

### `domainMatch` / `pathMatch` / `defaultPath` / `canonicalHost` / `expiry`
The RFC-6265 §5.1.3/§5.1.4/§5.3 matching primitives — domain-match (host-only =
exact), path-match (prefix at a `/` boundary), default-path derivation, host
canonicalisation (lower-case, trailing-dot strip), and Max-Age-over-Expires
lifetime resolution.
