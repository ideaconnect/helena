package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// rfc7616Challenge is the MD5 challenge from RFC 7616 §3.9.1.
const rfc7616Challenge = `Digest realm="http-auth@example.org", qop="auth, auth-int", ` +
	`algorithm=MD5, nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", ` +
	`opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`

// TestDigestAuthorizeRFC7616MD5 pins the response hash against the RFC 7616
// §3.9.1 worked example (independently recomputed via Python md5).
func TestDigestAuthorizeRFC7616MD5(t *testing.T) {
	c := parseDigestChallenge(rfc7616Challenge)
	d := model.DigestAuth{Username: "Mufasa", Password: "Circle of Life"}
	got, err := digestAuthorize(c, "GET", "/dir/index.html", d,
		"f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `response="8ca523f5e9506fed4657c9700eebdbec"`) {
		t.Errorf("MD5 response wrong:\n%s", got)
	}
	for _, want := range []string{`username="Mufasa"`, `realm="http-auth@example.org"`,
		"algorithm=MD5", "qop=auth", "nc=00000001",
		`opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`} {
		if !strings.Contains(got, want) {
			t.Errorf("header missing %q:\n%s", want, got)
		}
	}
}

// TestDigestAuthorizeSHA256 pins the SHA-256 variant from the same RFC example.
func TestDigestAuthorizeSHA256(t *testing.T) {
	c := parseDigestChallenge(strings.Replace(rfc7616Challenge, "algorithm=MD5", "algorithm=SHA-256", 1))
	d := model.DigestAuth{Username: "Mufasa", Password: "Circle of Life"}
	got, err := digestAuthorize(c, "GET", "/dir/index.html", d,
		"f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1"
	if !strings.Contains(got, `response="`+want+`"`) {
		t.Errorf("SHA-256 response wrong:\n%s", got)
	}
}

// TestDigestLegacyRFC2069 covers the no-qop (RFC 2069) form: no nc/cnonce/qop in
// the header and KD = H(HA1:nonce:HA2).
func TestDigestLegacyRFC2069(t *testing.T) {
	c := parseDigestChallenge(`Digest realm="r", nonce="abc"`)
	got, err := digestAuthorize(c, "GET", "/x", model.DigestAuth{Username: "u", Password: "p"}, "cn", 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "qop=") || strings.Contains(got, "nc=") {
		t.Errorf("legacy form should not carry qop/nc: %s", got)
	}
}

// TestDigestRespondPicksDigest verifies DigestRespond ignores a Basic challenge,
// answers the Digest one, and returns ok=false when only auth-int is offered.
func TestDigestRespondPicksDigest(t *testing.T) {
	d := model.DigestAuth{Username: "u", Password: "p"}
	hdr, ok, err := DigestRespond([]string{"Basic realm=\"x\"", rfc7616Challenge}, "GET", "/dir/index.html", d)
	if err != nil || !ok {
		t.Fatalf("expected a digest answer, ok=%v err=%v", ok, err)
	}
	if !strings.HasPrefix(hdr, "Digest ") {
		t.Errorf("header = %q", hdr)
	}

	if _, ok, _ := DigestRespond([]string{"Basic realm=\"x\""}, "GET", "/x", d); ok {
		t.Error("no Digest challenge present — ok should be false")
	}
	if _, ok, err := DigestRespond([]string{`Digest realm="r", nonce="n", qop="auth-int"`}, "GET", "/x", d); err == nil && ok {
		t.Error("auth-int only should not produce a header")
	}
}

// TestApplyDigestNoop verifies Apply emits nothing for Digest (the challenge
// round is the httpclient's job) and ResolveValues substitutes {{vars}}.
func TestApplyDigestNoop(t *testing.T) {
	a := ResolveValues(model.Auth{Type: model.AuthDigest, Digest: &model.DigestAuth{Username: "{{u}}", Password: "p"}},
		func(s string) string {
			if s == "{{u}}" {
				return "alice"
			}
			return s
		})
	if a.Digest.Username != "alice" {
		t.Errorf("username not resolved: %q", a.Digest.Username)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("Digest Apply should not set Authorization on the first request")
	}
}
