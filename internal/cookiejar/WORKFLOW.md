# cookiejar — Workflow

## How a cookie travels through a Send

The jar is never called directly by Helena's request code — `net/http` drives it
because the per-send `*http.Client` has the jar installed as its `Jar`
([internal/httpclient](../httpclient) `SetCookieJar`, wired in
[internal/ui/send.go](../ui/send.go)). For one Send:

1. Before the request goes on the wire, `net/http` calls `Cookies(req.URL)` and
   appends each returned cookie to the request's `Cookie` header. A `Cookie`
   header the user set explicitly is preserved — jar cookies are added to it, not
   substituted.
2. When the response (or any redirect hop) carries `Set-Cookie`, `net/http` calls
   `SetCookies(u, cookies)`, which stores/updates/deletes the jar entries.
3. Across a chain run the same jar instance is shared by every step's Client, so
   a login step's `Set-Cookie` is available to the steps and the leaf that follow
   — the core acceptance behaviour of #91.

Because the jar lives at session scope, the same flow carries cookies across
*separate* Sends too, for as long as the app is running.

## SetCookies — storing a response cookie

For each cookie, in order:

1. Drop it if the URL is nil/host-less or the cookie has no name.
2. `domainAttr(host, c.Domain)` decides the cookie's domain and host-only flag,
   and **rejects** cookies the response host can't set: a `Domain` that isn't the
   host or a parent of it, or a dotless super-cookie domain like `com`.
3. The path is the cookie's `Path` (if absolute) or the request URL's default
   path (`defaultPath`).
4. `expiry` resolves the lifetime: `Max-Age` wins over `Expires`; a negative
   `Max-Age` or an `Expires` already in the past marks a **deletion** (the slot
   is removed); otherwise a future instant, or zero for a session cookie.
5. `store` upserts at `key(domain, path, name)`, preserving the original
   `created`/`seq` on an update so ordering is stable.

## Cookies — choosing what to send

`Cookies(u)` filters the live entries to those that domain-match `u`'s host,
path-match `u`'s path, and (for `Secure` cookies) are being sent over https. It
purges any expired entry it passes. Survivors are sorted **longest path first,
then oldest** (RFC 6265 §5.4) and returned as bare `name=value` cookies.

## Viewer/editor edits

The UI ([internal/ui/cookies.go](../ui/cookies.go)) reads `All()` (a sorted live
snapshot) to populate its list, and mutates the live jar directly:

- **Add / Edit** → `Set(c)`. Edit removes the old `(domain, path, name)` slot
  first, because changing any of those three is an identity change, not an
  in-place update.
- **Delete** → `Remove(domain, path, name)`.
- **Clear all** → `Clear()`.

Every edit hits the same jar the next Send will read, so changes take effect
immediately with no save step.

## Expiry & purging

There is no background sweeper. Expired cookies are removed lazily the next time
`Cookies`, `All`, or `Len` walks the map (all share `liveEntries`). A session
cookie (zero `Expires`) never expires on its own — it lives until the app exits
or the user clears it.

## Concurrency

A Send runs on a worker goroutine while the viewer reads on the UI goroutine, so
every public method takes `mu` (read+write methods all need the write lock, since
even the "read" paths purge expired entries). `TestConcurrentSafe` hammers
`SetCookies`/`Cookies`/`All` together under `-race`.
