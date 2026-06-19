package cookiejar

import (
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

// compile-time check: Jar satisfies the stdlib interface so an *http.Client can
// use it directly.
var _ http.CookieJar = (*Jar)(nil)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// names returns the names of the cookies Cookies(u) yields, in order.
func names(cs []*http.Cookie) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestSetAndSendBasic(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{{Name: "sid", Value: "abc"}})

	got := j.Cookies(mustURL(t, "https://example.com/anything"))
	if len(got) != 1 || got[0].Name != "sid" || got[0].Value != "abc" {
		t.Fatalf("want sid=abc, got %+v", got)
	}
	if n := j.Len(); n != 1 {
		t.Fatalf("Len=%d want 1", n)
	}
}

func TestHostOnlyDoesNotLeakToOtherHosts(t *testing.T) {
	j := New()
	// No Domain attribute → host-only, must NOT go to sub-domains or siblings.
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{{Name: "a", Value: "1"}})
	if got := j.Cookies(mustURL(t, "https://api.example.com/")); len(got) != 0 {
		t.Fatalf("host-only cookie leaked to subdomain: %v", names(got))
	}
	if got := j.Cookies(mustURL(t, "https://other.com/")); len(got) != 0 {
		t.Fatalf("host-only cookie leaked to other host: %v", names(got))
	}
	if got := j.Cookies(mustURL(t, "https://example.com/")); len(got) != 1 {
		t.Fatalf("host-only cookie not returned to its own host: %v", names(got))
	}
}

func TestDomainCookieReachesSubdomains(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{{Name: "a", Value: "1", Domain: ".example.com"}})
	for _, host := range []string{"https://example.com/", "https://api.example.com/", "https://deep.api.example.com/"} {
		if got := j.Cookies(mustURL(t, host)); len(got) != 1 {
			t.Fatalf("domain cookie missing at %s: %v", host, names(got))
		}
	}
}

func TestRejectsForeignAndSuperCookieDomains(t *testing.T) {
	j := New()
	host := mustURL(t, "https://example.com/")
	j.SetCookies(host, []*http.Cookie{
		{Name: "foreign", Value: "x", Domain: "evil.com"}, // not a parent of example.com
		{Name: "super", Value: "x", Domain: "com"},        // dotless super-cookie
	})
	if got := j.Cookies(host); len(got) != 0 {
		t.Fatalf("foreign/super cookies were accepted: %v", names(got))
	}
}

func TestIPHostCookiesAreHostOnly(t *testing.T) {
	j := New()
	// A crafted dotted-suffix Domain from an IP host must NOT leak to sibling IPs
	// (10.0.0.1 setting Domain=0.0.1 would otherwise reach 20.0.0.1). IPs have no
	// sub-domains, so only an exact-host Domain is accepted and it stays host-only.
	j.SetCookies(mustURL(t, "http://10.0.0.1/"), []*http.Cookie{
		{Name: "leak", Value: "1", Domain: "0.0.1"},    // crafted suffix → rejected
		{Name: "leak2", Value: "1", Domain: ".0.0.1"},  // leading-dot variant → rejected
		{Name: "host", Value: "1", Domain: "10.0.0.1"}, // exact IP → accepted, host-only
		{Name: "plain", Value: "1"},                    // no attribute → host-only
	})
	if got := j.Cookies(mustURL(t, "http://20.0.0.1/")); len(got) != 0 {
		t.Fatalf("IP cookie leaked to sibling IP: %v", names(got))
	}
	got := names(j.Cookies(mustURL(t, "http://10.0.0.1/")))
	if len(got) != 2 {
		t.Fatalf("exact-IP + plain cookies should be sent to their own host, got %v", got)
	}
}

func TestSingleLabelHostCookiesAreHostOnly(t *testing.T) {
	j := New()
	// A single-label host (localhost/intranet) setting Domain=itself must be
	// host-only — not a domain cookie that would leak to evil.localhost.
	j.SetCookies(mustURL(t, "http://localhost/"), []*http.Cookie{{Name: "a", Value: "1", Domain: "localhost"}})
	if got := j.Cookies(mustURL(t, "http://evil.localhost/")); len(got) != 0 {
		t.Fatalf("single-label domain cookie leaked to subdomain: %v", names(got))
	}
	if got := j.Cookies(mustURL(t, "http://localhost/")); len(got) != 1 {
		t.Fatalf("single-label cookie not returned to its own host: %v", names(got))
	}
	// A dotted DNS Domain still produces a normal domain cookie (reaches subs).
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{{Name: "b", Value: "1", Domain: "example.com"}})
	if got := j.Cookies(mustURL(t, "https://api.example.com/")); len(got) != 1 {
		t.Fatalf("dotted domain cookie should still reach subdomains: %v", names(got))
	}
}

func TestZonedIPv6HostStaysHostOnly(t *testing.T) {
	j := New()
	// A zoned IPv6 host (fe80::1%eth0) must be detected as an IP despite the
	// %zone, so a Domain attribute is accepted only as the exact host and the
	// cookie stays host-only.
	u := mustURL(t, "http://[fe80::1%25eth0]/")
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1", Domain: "fe80::1%eth0"}})
	all := j.All()
	if len(all) != 1 || !all[0].HostOnly {
		t.Fatalf("zoned IPv6 cookie should be stored host-only: %+v", all)
	}
	if got := j.Cookies(u); len(got) != 1 {
		t.Fatalf("zoned IPv6 cookie not returned to its own host: %v", names(got))
	}
}

func TestSetForcesIPHostOnlyAndRejectsNUL(t *testing.T) {
	j := New()
	// Manual Set with an IP domain must be forced host-only so the editor can't
	// recreate the IP leak the wire path guards against.
	j.Set(Cookie{Domain: "10.0.0.1", Name: "x", Value: "1"})
	all := j.All()
	if len(all) != 1 || !all[0].HostOnly {
		t.Fatalf("Set with IP domain should be host-only: %+v", all)
	}
	if got := j.Cookies(mustURL(t, "http://evil.10.0.0.1/")); len(got) != 0 {
		t.Fatalf("IP cookie set via Set leaked to a suffixed host: %v", names(got))
	}
	// A NUL in name or path is rejected — it would corrupt the key separator and
	// could alias a different slot.
	j.Set(Cookie{Domain: "x.com", Name: "a\x00b", Value: "1"})
	j.Set(Cookie{Domain: "x.com", Name: "a", Path: "/p\x00q", Value: "1"})
	if j.Len() != 1 {
		t.Fatalf("NUL-bearing Set should be a no-op, Len=%d", j.Len())
	}
}

func TestReplacePreservesOrderAndIdentity(t *testing.T) {
	j := New()
	u := mustURL(t, "https://example.com/")
	j.Set(Cookie{Domain: "example.com", Path: "/", Name: "a", Value: "1"})
	j.Set(Cookie{Domain: "example.com", Path: "/", Name: "b", Value: "1"})

	// A value-only edit (identity unchanged) must keep send-order position.
	j.Replace("example.com", "/", "a", Cookie{Domain: "example.com", Path: "/", Name: "a", Value: "2"})
	if got := names(j.Cookies(u)); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("value-only edit reordered cookies: %v", got)
	}
	for _, c := range j.Cookies(u) {
		if c.Name == "a" && c.Value != "2" {
			t.Fatalf("value-only edit lost the new value: %+v", c)
		}
	}

	// An identity change (rename a→c) drops the old slot (no orphan).
	j.Replace("example.com", "/", "a", Cookie{Domain: "example.com", Path: "/", Name: "c", Value: "2"})
	if j.Len() != 2 {
		t.Fatalf("rename via Replace orphaned a slot, Len=%d", j.Len())
	}
	for _, n := range names(j.Cookies(u)) {
		if n == "a" {
			t.Fatal("old name 'a' still present after rename")
		}
	}

	// Replace with an invalid cookie (no name) is a no-op.
	j.Replace("example.com", "/", "b", Cookie{Domain: "example.com", Path: "/", Name: ""})
	if j.Len() != 2 {
		t.Fatalf("invalid Replace should be a no-op, Len=%d", j.Len())
	}
}

func TestSecureCookieOnlyOverHTTPS(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{{Name: "s", Value: "1", Secure: true}})
	if got := j.Cookies(mustURL(t, "http://example.com/")); len(got) != 0 {
		t.Fatalf("secure cookie sent over http: %v", names(got))
	}
	if got := j.Cookies(mustURL(t, "https://example.com/")); len(got) != 1 {
		t.Fatalf("secure cookie not sent over https: %v", names(got))
	}
}

func TestPathScopingAndOrdering(t *testing.T) {
	j := New()
	u := mustURL(t, "https://example.com/a/b")
	// Default path of /a/b is /a; an explicit deeper path is more specific.
	j.SetCookies(mustURL(t, "https://example.com/a/b"), []*http.Cookie{{Name: "shallow", Value: "1"}})
	j.SetCookies(u, []*http.Cookie{{Name: "deep", Value: "1", Path: "/a/b"}})
	j.SetCookies(u, []*http.Cookie{{Name: "root", Value: "1", Path: "/"}})

	got := names(j.Cookies(mustURL(t, "https://example.com/a/b")))
	// Longest path first: /a/b (deep), then /a (shallow), then / (root).
	want := []string{"deep", "shallow", "root"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v", got, want)
		}
	}
	// A request that doesn't match the deep path drops "deep".
	if got := names(j.Cookies(mustURL(t, "https://example.com/a/x"))); len(got) != 2 {
		t.Fatalf("path scoping wrong at /a/x: %v", got)
	}
}

func TestMaxAgeDeleteAndExpiry(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	j := New()
	j.now = func() time.Time { return base }
	u := mustURL(t, "https://example.com/")

	j.SetCookies(u, []*http.Cookie{{Name: "keep", Value: "1", MaxAge: 3600}})
	j.SetCookies(u, []*http.Cookie{{Name: "gone", Value: "1"}})
	// MaxAge<0 deletes an existing cookie.
	j.SetCookies(u, []*http.Cookie{{Name: "gone", Value: "1", MaxAge: -1}})
	if got := names(j.Cookies(u)); len(got) != 1 || got[0] != "keep" {
		t.Fatalf("after delete: %v", got)
	}

	// Advance past the Max-Age → keep expires and is purged.
	j.now = func() time.Time { return base.Add(2 * time.Hour) }
	if got := j.Cookies(u); len(got) != 0 {
		t.Fatalf("expired cookie still sent: %v", names(got))
	}
	if j.Len() != 0 {
		t.Fatalf("expired cookie not purged, Len=%d", j.Len())
	}
}

func TestExpiresAttribute(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	j := New()
	j.now = func() time.Time { return base }
	u := mustURL(t, "https://example.com/")

	// Expires in the past → immediate delete (no-op insert).
	j.SetCookies(u, []*http.Cookie{{Name: "past", Value: "1", Expires: base.Add(-time.Hour)}})
	// Expires in the future → stored with that expiry.
	j.SetCookies(u, []*http.Cookie{{Name: "future", Value: "1", Expires: base.Add(time.Hour)}})
	if got := names(j.Cookies(u)); len(got) != 1 || got[0] != "future" {
		t.Fatalf("expires handling: %v", got)
	}

	all := j.All()
	if len(all) != 1 || all[0].Expires.IsZero() {
		t.Fatalf("future cookie should carry a non-zero Expires: %+v", all)
	}
}

func TestUpdatePreservesSlotAndOrder(t *testing.T) {
	j := New()
	u := mustURL(t, "https://example.com/")
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1"}})
	j.SetCookies(u, []*http.Cookie{{Name: "b", Value: "1"}})
	// Re-set a: same (domain,path,name) slot → value updated, no duplicate.
	j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "2"}})

	got := j.Cookies(u)
	if len(got) != 2 {
		t.Fatalf("update created a duplicate: %v", names(got))
	}
	for _, c := range got {
		if c.Name == "a" && c.Value != "2" {
			t.Fatalf("update did not change value: %+v", c)
		}
	}
}

func TestManualSetEditRemoveClear(t *testing.T) {
	j := New()
	j.Set(Cookie{Domain: ".Example.COM.", Name: "token", Value: "v", Path: ""})
	all := j.All()
	if len(all) != 1 {
		t.Fatalf("manual Set: %+v", all)
	}
	if all[0].Domain != "example.com" || all[0].Path != "/" {
		t.Fatalf("Set should canonicalise domain and default path: %+v", all[0])
	}
	// It should now be sent.
	if got := j.Cookies(mustURL(t, "https://example.com/")); len(got) != 1 {
		t.Fatalf("manually set cookie not sent: %v", names(got))
	}

	// Invalid manual sets are no-ops.
	j.Set(Cookie{Name: "noDomain"})
	j.Set(Cookie{Domain: "x.com"}) // no name
	if j.Len() != 1 {
		t.Fatalf("invalid Set was not a no-op, Len=%d", j.Len())
	}

	// Remove with a leading-dot domain + empty path still resolves to the slot.
	j.Remove(".example.com", "", "token")
	if j.Len() != 0 {
		t.Fatalf("Remove failed, Len=%d", j.Len())
	}

	j.Set(Cookie{Domain: "a.com", Name: "x", Value: "1"})
	j.Set(Cookie{Domain: "b.com", Name: "y", Value: "1"})
	j.Clear()
	if j.Len() != 0 {
		t.Fatalf("Clear failed, Len=%d", j.Len())
	}
}

func TestAllSortedStably(t *testing.T) {
	j := New()
	j.Set(Cookie{Domain: "b.com", Name: "z", Value: "1"})
	j.Set(Cookie{Domain: "a.com", Name: "y", Value: "1", Path: "/x"})
	j.Set(Cookie{Domain: "a.com", Name: "x", Value: "1", Path: "/x"})
	j.Set(Cookie{Domain: "a.com", Name: "w", Value: "1", Path: "/"})

	got := j.All()
	want := []struct{ d, p, n string }{
		{"a.com", "/", "w"},
		{"a.com", "/x", "x"},
		{"a.com", "/x", "y"},
		{"b.com", "/", "z"},
	}
	if len(got) != len(want) {
		t.Fatalf("len got %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Domain != w.d || got[i].Path != w.p || got[i].Name != w.n {
			t.Fatalf("All[%d] = %s %s %s, want %s %s %s", i, got[i].Domain, got[i].Path, got[i].Name, w.d, w.p, w.n)
		}
	}
}

func TestSetCookiesGuards(t *testing.T) {
	j := New()
	// nil URL, empty list, hostless URL, nil/anonymous cookie — all no-ops.
	j.SetCookies(nil, []*http.Cookie{{Name: "x", Value: "1"}})
	j.SetCookies(mustURL(t, "https://example.com/"), nil)
	j.SetCookies(mustURL(t, "file:///tmp/x"), []*http.Cookie{{Name: "x", Value: "1"}})
	j.SetCookies(mustURL(t, "https://example.com/"), []*http.Cookie{nil, {Name: "", Value: "1"}})
	if j.Len() != 0 {
		t.Fatalf("guards failed, Len=%d", j.Len())
	}
	// Cookies with nil/hostless URL → nil.
	if j.Cookies(nil) != nil {
		t.Fatal("Cookies(nil) should be nil")
	}
	if j.Cookies(mustURL(t, "file:///tmp/x")) != nil {
		t.Fatal("Cookies(hostless) should be nil")
	}
}

func TestUnitHelpers(t *testing.T) {
	if got := defaultPath("/a/b/c"); got != "/a/b" {
		t.Fatalf("defaultPath nested = %q", got)
	}
	if got := defaultPath("/a"); got != "/" {
		t.Fatalf("defaultPath single = %q", got)
	}
	if got := defaultPath("noslash"); got != "/" {
		t.Fatalf("defaultPath relative = %q", got)
	}
	if got := defaultPath(""); got != "/" {
		t.Fatalf("defaultPath empty = %q", got)
	}
	if !pathMatch("/a/b", "/a/") {
		t.Fatal("pathMatch trailing-slash prefix")
	}
	if !pathMatch("/a/b", "/a") {
		t.Fatal("pathMatch boundary prefix")
	}
	if pathMatch("/ab", "/a") {
		t.Fatal("pathMatch must not match non-boundary prefix")
	}
	if got := canonicalHost("  EXAMPLE.com.  "); got != "example.com" {
		t.Fatalf("canonicalHost = %q", got)
	}
	if !domainMatch("example.com", "example.com", true) {
		t.Fatal("domainMatch exact host-only")
	}
	if domainMatch("api.example.com", "example.com", true) {
		t.Fatal("domainMatch host-only must reject subdomain")
	}
}

// TestConcurrentSafe hammers reads and writes so -race proves the locking.
func TestConcurrentSafe(t *testing.T) {
	j := New()
	u := mustURL(t, "https://example.com/")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); j.SetCookies(u, []*http.Cookie{{Name: "a", Value: "1"}}) }()
		go func() { defer wg.Done(); _ = j.Cookies(u) }()
		go func() { defer wg.Done(); _ = j.All() }()
	}
	wg.Wait()
}
