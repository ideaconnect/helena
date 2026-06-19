// Package cookiejar implements an observable, editable HTTP cookie jar for
// Helena (#91). It satisfies net/http's CookieJar interface so the per-send
// *http.Client stores Set-Cookie responses and replays matching cookies on
// subsequent requests — including across redirects and across the throwaway
// per-send Clients the UI builds, because the jar is held at session scope
// (mirroring the cached transport, #52).
//
// Unlike net/http/cookiejar, this jar exposes its contents (All) and lets the
// UI edit them (Set / Remove / Clear), which a cookie viewer/editor needs:
// the stdlib jar's Cookies() returns only name=value and hides everything else.
//
// Scope & persistence: the jar is in-memory and session-lifetime only. Cookies
// accumulate across sends while Helena runs and are dropped on exit; they are
// never written to disk, so session credentials can't leak into a file or a
// git-tracked collection. See WORKFLOW.md for the rationale.
//
// Matching follows the practical subset of RFC 6265 (domain-match, path-match,
// Secure, host-only, Max-Age/Expires). IP-literal hosts are treated as the
// stdlib jar does: a Domain attribute is honoured only when it equals the host,
// and the cookie is always host-only (so it can't leak across sibling IPs). It
// does NOT consult the public-suffix list, so it cannot reject a DNS cookie
// scoped to a registry suffix like "co.uk"; it does reject obvious super-cookies
// (a dotless Domain that isn't the exact host, e.g. "com"). For a desktop client
// whose trust model already runs collection scripts with the user's network
// identity, that trade-off is acceptable; net/http/cookiejar + publicsuffix.List
// is the upgrade path.
package cookiejar

import (
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cookie is a stored cookie together with the attributes the viewer/editor
// surfaces. Expires is zero for a session cookie (which this jar keeps for the
// process lifetime, since nothing is persisted anyway).
type Cookie struct {
	Name     string
	Value    string
	Domain   string    // host the cookie is scoped to, canonical (lower-case, no leading dot)
	Path     string    // URL path prefix the cookie applies to
	Expires  time.Time // zero ⇒ session cookie
	Secure   bool      // only sent over https
	HTTPOnly bool      // advisory here (no JS surface); preserved for display/round-trip
	HostOnly bool      // set without a Domain attribute ⇒ exact-host match only
}

// entry pairs a stored Cookie with bookkeeping the public type omits: creation
// time (RFC 6265 keeps it stable across updates) and a monotonic sequence used
// as the stable tiebreaker when two cookies have equal path length / created.
type entry struct {
	Cookie
	created time.Time
	seq     int64
}

// Jar is a concurrency-safe cookie jar. Sends run on a worker goroutine while
// the viewer reads on the UI goroutine, so every accessor takes the mutex.
type Jar struct {
	mu  sync.RWMutex
	m   map[string]entry
	seq int64
	// now is the clock, swappable in tests to exercise expiry deterministically.
	now func() time.Time
}

// New returns an empty jar using the wall clock.
func New() *Jar {
	return &Jar{m: map[string]entry{}, now: time.Now}
}

// key identifies a cookie slot. Per RFC 6265 a cookie is uniquely keyed by the
// (domain, path, name) triple — a later Set-Cookie with the same triple updates
// the existing value rather than adding a duplicate.
func key(domain, path, name string) string {
	return domain + "\x00" + path + "\x00" + name
}

// SetCookies stores the response cookies for the request URL u. It implements
// http.CookieJar, so net/http calls it for every Set-Cookie header (on the
// original response and on each redirect hop).
func (j *Jar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u == nil || len(cookies) == 0 {
		return
	}
	host := canonicalHost(u.Hostname())
	if host == "" {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.now()
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		domain, hostOnly, ok := domainAttr(host, c.Domain)
		if !ok {
			continue // cookie scoped to a domain the response host can't set
		}
		path := c.Path
		if path == "" || path[0] != '/' {
			path = defaultPath(u.Path)
		}
		expires, deleteNow := expiry(c, now)
		k := key(domain, path, c.Name)
		if deleteNow {
			delete(j.m, k)
			continue
		}
		e := entry{Cookie: Cookie{
			Name: c.Name, Value: c.Value, Domain: domain, Path: path,
			Expires: expires, Secure: c.Secure, HTTPOnly: c.HttpOnly, HostOnly: hostOnly,
		}}
		j.store(k, e, now)
	}
}

// Cookies returns the cookies to send with a request to u, ordered per RFC 6265
// §5.4 (longest path first, then oldest). It implements http.CookieJar.
// Expired cookies are purged as a side effect.
func (j *Jar) Cookies(u *url.URL) []*http.Cookie {
	if u == nil {
		return nil
	}
	host := canonicalHost(u.Hostname())
	if host == "" {
		return nil
	}
	secure := strings.EqualFold(u.Scheme, "https")
	reqPath := u.Path
	if reqPath == "" {
		reqPath = "/"
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	matches := j.liveEntries(func(e entry) bool {
		if e.Secure && !secure {
			return false
		}
		return domainMatch(host, e.Domain, e.HostOnly) && pathMatch(reqPath, e.Path)
	})
	sort.Slice(matches, func(a, b int) bool {
		if len(matches[a].Path) != len(matches[b].Path) {
			return len(matches[a].Path) > len(matches[b].Path)
		}
		return matches[a].seq < matches[b].seq
	})
	out := make([]*http.Cookie, len(matches))
	for i, e := range matches {
		out[i] = &http.Cookie{Name: e.Name, Value: e.Value}
	}
	return out
}

// All returns a snapshot of every live cookie, sorted (domain, path, name) for
// stable display. Expired cookies are purged as a side effect.
func (j *Jar) All() []Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	entries := j.liveEntries(nil)
	out := make([]Cookie, len(entries))
	for i, e := range entries {
		out[i] = e.Cookie
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Domain != out[b].Domain {
			return out[a].Domain < out[b].Domain
		}
		if out[a].Path != out[b].Path {
			return out[a].Path < out[b].Path
		}
		return out[a].Name < out[b].Name
	})
	return out
}

// Len reports how many live cookies the jar holds (expired ones excluded).
func (j *Jar) Len() int {
	return len(j.All())
}

// Set upserts a cookie directly — the path the viewer's add uses. Domain and
// Name are required (anything else is a no-op); an empty Path defaults to "/".
// The cookie is normalised exactly as a wire cookie would be (canonical domain,
// IP domains forced host-only), so the editor can't create a scope the wire path
// wouldn't.
func (j *Jar) Set(c Cookie) {
	nc, ok := normalize(c)
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.store(key(nc.Domain, nc.Path, nc.Name), entry{Cookie: nc}, j.now())
}

// Replace updates the cookie identified by (oldDomain, oldPath, oldName) to c.
// When the new identity is unchanged it preserves the cookie's send-order
// position (a value- or flag-only edit must not reorder it relative to its
// equal-path-length peers, RFC 6265 §5.4); when the identity changed the old
// slot is removed so an edit can't leave an orphan duplicate. This is the path
// the viewer's edit uses.
func (j *Jar) Replace(oldDomain, oldPath, oldName string, c Cookie) {
	nc, ok := normalize(c)
	if !ok {
		return
	}
	if oldPath == "" {
		oldPath = "/"
	}
	oldKey := key(canonicalHost(strings.TrimPrefix(oldDomain, ".")), oldPath, oldName)
	newKey := key(nc.Domain, nc.Path, nc.Name)
	j.mu.Lock()
	defer j.mu.Unlock()
	if oldKey != newKey {
		delete(j.m, oldKey) // identity changed → drop the old slot (no orphan)
	}
	// store preserves created/seq when newKey already exists, so a same-identity
	// edit keeps its position; a changed identity gets a fresh seq.
	j.store(newKey, entry{Cookie: nc}, j.now())
}

// normalize canonicalises and validates a cookie for storage, shared by Set and
// Replace so every editor entry path enforces the same rules as the wire path:
// required Domain+Name, default Path, no NUL (it would corrupt the key
// separator), and IP-literal domains forced host-only (an IP has no
// sub-domains, so a domain-scoped IP cookie would leak across sibling IPs).
func normalize(c Cookie) (Cookie, bool) {
	c.Domain = canonicalHost(strings.TrimPrefix(c.Domain, "."))
	if c.Domain == "" || c.Name == "" {
		return c, false
	}
	if strings.ContainsRune(c.Name, 0) || strings.ContainsRune(c.Path, 0) || strings.ContainsRune(c.Domain, 0) {
		return c, false
	}
	if c.Path == "" {
		c.Path = "/"
	}
	if isIP(c.Domain) {
		c.HostOnly = true
	}
	return c, true
}

// Remove deletes the cookie identified by (domain, path, name); an empty path
// is treated as "/". A missing key is a no-op.
func (j *Jar) Remove(domain, path, name string) {
	if path == "" {
		path = "/"
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.m, key(canonicalHost(strings.TrimPrefix(domain, ".")), path, name))
}

// Clear empties the jar.
func (j *Jar) Clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.m = map[string]entry{}
}

// store inserts/updates e at k, preserving the original creation time and seq on
// an update so ordering stays stable. Caller holds the lock.
func (j *Jar) store(k string, e entry, now time.Time) {
	if old, ok := j.m[k]; ok {
		e.created = old.created
		e.seq = old.seq
	} else {
		j.seq++
		e.created = now
		e.seq = j.seq
	}
	j.m[k] = e
}

// liveEntries returns the non-expired entries matching keep (nil ⇒ all), purging
// expired ones it walks past. Caller holds the lock.
func (j *Jar) liveEntries(keep func(entry) bool) []entry {
	now := j.now()
	var out []entry
	for k, e := range j.m {
		if !e.Expires.IsZero() && !e.Expires.After(now) {
			delete(j.m, k)
			continue
		}
		if keep == nil || keep(e) {
			out = append(out, e)
		}
	}
	return out
}

// expiry resolves a wire cookie's lifetime: Max-Age wins over Expires (RFC
// 6265 §5.3). A negative Max-Age, or an Expires already in the past, signals a
// deletion; a zero result with deleteNow=false is a session cookie.
func expiry(c *http.Cookie, now time.Time) (expires time.Time, deleteNow bool) {
	switch {
	case c.MaxAge < 0:
		return time.Time{}, true
	case c.MaxAge > 0:
		return now.Add(time.Duration(c.MaxAge) * time.Second), false
	case !c.Expires.IsZero():
		if !c.Expires.After(now) {
			return time.Time{}, true
		}
		return c.Expires, false
	default:
		return time.Time{}, false
	}
}

// domainAttr resolves a Set-Cookie Domain attribute against the response host,
// returning the canonical cookie domain, whether it is host-only (no Domain
// attribute), and whether the host is allowed to set it. A Domain that the host
// doesn't domain-match, or a dotless super-cookie domain like "com", is rejected.
func domainAttr(host, attr string) (domain string, hostOnly, ok bool) {
	if attr == "" {
		return host, true, true
	}
	d := canonicalHost(strings.TrimPrefix(attr, "."))
	if d == "" {
		return "", false, false
	}
	// An IP-literal host has no sub-domains, so a Domain attribute is only valid
	// when it equals the host, and the cookie is always host-only (mirrors
	// net/http/cookiejar). Without this, a dotted IP like 10.0.0.1 would treat
	// Domain=0.0.1 as a valid suffix (it ends in ".0.0.1" and contains a dot, so
	// the guards below pass) and leak the cookie to every x.0.0.1 host.
	if isIP(host) {
		if d != host {
			return "", false, false
		}
		return host, true, true
	}
	if host != d && !strings.HasSuffix(host, "."+d) {
		return "", false, false // not the host or a parent of it
	}
	// A dotless Domain has no registrable parent: a "com"-style super-cookie
	// (host != d) is rejected, and a single-label host (e.g. "localhost",
	// "intranet") setting Domain=itself is accepted only as host-only — never as
	// a domain cookie, which would otherwise leak to sibling sub-domains like
	// "evil.localhost" (mirrors net/http/cookiejar).
	if !strings.Contains(d, ".") {
		if host != d {
			return "", false, false
		}
		return d, true, true
	}
	return d, false, true
}

// isIP reports whether host is an IP literal, tolerating an IPv6 zone id
// (e.g. "fe80::1%eth0", which url.Hostname returns unbracketed). net.ParseIP
// alone returns nil for a zoned address, which would let it slip through the
// IP-host guard in domainAttr as a DNS name.
func isIP(host string) bool {
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return net.ParseIP(host) != nil
}

// domainMatch reports whether a request to host should receive a cookie scoped
// to domain. Host-only cookies require an exact match; others match the domain
// itself or any sub-domain of it (RFC 6265 §5.1.3).
func domainMatch(host, domain string, hostOnly bool) bool {
	if host == domain {
		return true
	}
	if hostOnly {
		return false
	}
	return strings.HasSuffix(host, "."+domain)
}

// pathMatch implements RFC 6265 §5.1.4 path-match: equal paths match; otherwise
// the cookie path must be a prefix of the request path ending at a "/" boundary.
func pathMatch(reqPath, cookiePath string) bool {
	if reqPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(reqPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || reqPath[len(cookiePath)] == '/'
}

// defaultPath derives the cookie's default path from the request path (RFC 6265
// §5.1.4): the substring up to (not including) the rightmost "/", or "/".
func defaultPath(p string) string {
	if p == "" || p[0] != '/' {
		return "/"
	}
	i := strings.LastIndex(p, "/")
	if i == 0 {
		return "/"
	}
	return p[:i]
}

// canonicalHost lower-cases a host and strips a trailing dot so cookies key and
// match consistently regardless of how the URL was written.
func canonicalHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
