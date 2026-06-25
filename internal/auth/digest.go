package auth

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/idct/helena/internal/model"
)

// DigestRespond answers an HTTP Digest 401 challenge (#75). It scans the
// response's WWW-Authenticate header values for a Digest challenge and, when one
// is present, returns the Authorization header value to retry the request with.
// ok is false when there is no Digest challenge to answer (e.g. the server
// offered only Basic, or only the unsupported auth-int qop). method is the HTTP
// method and uri is the request-target (path + query) the retry will use.
func DigestRespond(wwwAuthenticate []string, method, uri string, d model.DigestAuth) (header string, ok bool, err error) {
	var challenge map[string]string
	for _, h := range wwwAuthenticate {
		if c := parseDigestChallenge(h); c != nil {
			challenge = c
			break
		}
	}
	if challenge == nil || challenge["nonce"] == "" {
		return "", false, nil
	}
	cnonce, err := randomCnonce()
	if err != nil {
		return "", false, err
	}
	hdr, err := digestAuthorize(challenge, method, uri, d, cnonce, 1)
	if err != nil {
		return "", false, err
	}
	return hdr, true, nil
}

// parseDigestChallenge parses one WWW-Authenticate header value. It returns the
// directive map for a "Digest …" challenge, or nil for any other scheme.
func parseDigestChallenge(header string) map[string]string {
	header = strings.TrimSpace(header)
	if len(header) < 7 || !strings.EqualFold(header[:6], "Digest") {
		return nil
	}
	out := map[string]string{}
	for _, part := range splitDigestParams(header[6:]) {
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(part[:eq]))
		v := strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// splitDigestParams splits a comma-separated directive list while keeping commas
// that appear inside double-quoted values (e.g. qop="auth,auth-int").
func splitDigestParams(s string) []string {
	var parts []string
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			b.WriteByte(c)
		case c == ',' && !inQuote:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		parts = append(parts, b.String())
	}
	return parts
}

// digestAuthorize computes the Authorization: Digest header value from a parsed
// challenge. cnonce and nc are supplied by the caller for deterministic testing.
// Supports algorithm MD5 / MD5-sess / SHA-256 / SHA-256-sess (default MD5) and
// qop=auth (or the legacy RFC 2069 no-qop form). auth-int is not supported.
func digestAuthorize(c map[string]string, method, uri string, d model.DigestAuth, cnonce string, nc int) (string, error) {
	realm := c["realm"]
	nonce := c["nonce"]
	algorithm := c["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}
	sess := strings.HasSuffix(strings.ToUpper(algorithm), "-SESS")
	var h func(string) string
	switch {
	case strings.HasPrefix(strings.ToUpper(algorithm), "SHA-256"):
		h = func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	case strings.HasPrefix(strings.ToUpper(algorithm), "MD5"):
		h = func(s string) string { sum := md5.Sum([]byte(s)); return hex.EncodeToString(sum[:]) }
	default:
		return "", fmt.Errorf("auth.Digest: unsupported algorithm %q", algorithm)
	}

	// Pick a usable qop. The server may offer a comma list; we support "auth".
	qop := ""
	for _, q := range strings.Split(c["qop"], ",") {
		if strings.TrimSpace(q) == "auth" {
			qop = "auth"
			break
		}
	}
	if c["qop"] != "" && qop == "" {
		return "", fmt.Errorf("auth.Digest: no supported qop in %q", c["qop"])
	}

	ha1 := h(d.Username + ":" + realm + ":" + d.Password)
	if sess {
		ha1 = h(ha1 + ":" + nonce + ":" + cnonce)
	}
	ha2 := h(method + ":" + uri)

	ncStr := fmt.Sprintf("%08x", nc)
	var response string
	if qop == "auth" {
		response = h(strings.Join([]string{ha1, nonce, ncStr, cnonce, qop, ha2}, ":"))
	} else {
		// RFC 2069 legacy form (no qop).
		response = h(ha1 + ":" + nonce + ":" + ha2)
	}

	parts := []string{
		fmt.Sprintf(`username=%q`, d.Username),
		fmt.Sprintf(`realm=%q`, realm),
		fmt.Sprintf(`nonce=%q`, nonce),
		fmt.Sprintf(`uri=%q`, uri),
		fmt.Sprintf(`response=%q`, response),
	}
	if algorithm != "" {
		parts = append(parts, "algorithm="+algorithm)
	}
	if qop == "auth" {
		parts = append(parts, "qop=auth", "nc="+ncStr, fmt.Sprintf(`cnonce=%q`, cnonce))
	}
	if op := c["opaque"]; op != "" {
		parts = append(parts, fmt.Sprintf(`opaque=%q`, op))
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

// randomCnonce returns a fresh 16-byte hex client nonce.
func randomCnonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth.Digest: cnonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}
