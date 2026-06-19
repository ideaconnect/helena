# cookiejar

An observable, editable HTTP cookie jar (#91). It satisfies `net/http`'s
`http.CookieJar` interface, so the per-send `*http.Client` automatically stores
`Set-Cookie` responses and replays matching cookies on later requests (including
across redirect hops). Unlike `net/http/cookiejar`, it also **exposes and lets
you edit** its contents — which Helena's cookie viewer/editor needs, because the
stdlib jar's `Cookies()` returns only `name=value` and hides domain/path/expiry.

The jar is held at **session scope** by [`internal/session`](../session) and
installed on every throwaway per-send `Client` (mirroring the cached transport,
#52), so cookies persist across sends — within a chain run and across runs while
the app is open.

## Scope & persistence (decision)

The jar is **in-memory and session-lifetime only**. Cookies accumulate while
Helena runs and are dropped on exit; they are **never written to disk**. This
keeps session credentials (the common case for cookies) out of any file and out
of git-tracked collections, consistent with the secret-externalization invariant
(#42). A persisted jar is a possible future enhancement, but the security cost of
writing session tokens to disk is why this first cut stays in memory.

## Matching (RFC 6265 subset)

`Cookies(u)` returns cookies whose domain, path, and Secure flag match `u`,
ordered longest-path-first then oldest (RFC 6265 §5.4). `SetCookies` honours the
`Domain` attribute (host-only when absent), the `Path` attribute (default path
derived from the request URL when absent), `Secure`, `HttpOnly`, and
`Max-Age`/`Expires` (Max-Age wins; a negative Max-Age or past Expires deletes).

IP-literal hosts are handled like the stdlib jar: a `Domain` attribute is
honoured only when it equals the host, and the cookie is always host-only, so a
crafted dotted-suffix `Domain` (e.g. `10.0.0.1` setting `Domain=0.0.1`) can't
leak to sibling IPs.

**Not implemented:** public-suffix-list checking. The jar rejects an obvious
super-cookie (a dotless `Domain` that isn't the exact host, e.g. `com`) but
cannot reject a registry suffix like `co.uk`. For a desktop client whose trust
model already runs collection scripts with the user's network identity (see
[internal/scripting](../scripting)), this is an accepted trade-off;
`net/http/cookiejar` + `golang.org/x/net/publicsuffix` is the upgrade path.

## Public API

### Types
- `Jar` — concurrency-safe cookie store implementing `http.CookieJar`.
- `Cookie` — a stored cookie with the attributes the viewer/editor surfaces
  (`Name`, `Value`, `Domain`, `Path`, `Expires`, `Secure`, `HTTPOnly`,
  `HostOnly`).

### Functions / methods
- `New() *Jar` — an empty jar using the wall clock.
- `(*Jar).SetCookies(u *url.URL, cookies []*http.Cookie)` — stores response
  cookies for `u` (the `http.CookieJar` write side).
- `(*Jar).Cookies(u *url.URL) []*http.Cookie` — the cookies to send to `u`,
  RFC-ordered (the `http.CookieJar` read side). Purges expired cookies it walks.
- `(*Jar).All() []Cookie` — a sorted snapshot (domain, path, name) of every live
  cookie, for the viewer.
- `(*Jar).Len() int` — count of live cookies.
- `(*Jar).Set(c Cookie)` — upsert a cookie directly (the editor's add/edit).
  Domain and Name are required; an empty Path defaults to `/`.
- `(*Jar).Remove(domain, path, name string)` — delete one cookie.
- `(*Jar).Clear()` — empty the jar.

## Dependencies

### Internal
None.

### External (standard library only)
- `net/http`, `net/url` — the `http.CookieJar` interface and URL parsing.
- `sort`, `strings`, `sync`, `time` — ordering, host/path handling, locking,
  expiry.
