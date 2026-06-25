package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/idct/helena/internal/model"
)

// oauth1Header builds the OAuth 1.0a `Authorization: OAuth …` header value for
// req (#77, RFC 5849). nonce and timestamp are injected so the caller controls
// them (Apply generates fresh ones; tests can pin them). The signature covers
// the oauth_* params, the URL query, and — for an application/x-www-form-
// urlencoded body — the body params, per the spec.
func oauth1Header(req *http.Request, a *model.OAuth1Auth, nonce, timestamp string) (string, error) {
	oauthParams := [][2]string{
		{"oauth_consumer_key", a.ConsumerKey},
		{"oauth_nonce", nonce},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", timestamp},
		{"oauth_version", "1.0"},
	}
	if a.Token != "" {
		oauthParams = append(oauthParams, [2]string{"oauth_token", a.Token})
	}

	params := append([][2]string(nil), oauthParams...)
	for k, vs := range req.URL.Query() {
		for _, v := range vs {
			params = append(params, [2]string{k, v})
		}
	}
	if formParams, err := oauth1FormBody(req); err != nil {
		return "", err
	} else {
		params = append(params, formParams...)
	}

	sig := oauth1Sign(req.Method, oauth1BaseURL(req.URL), params, a.ConsumerSecret, a.TokenSecret)

	header := append(oauthParams, [2]string{"oauth_signature", sig})
	sort.Slice(header, func(i, j int) bool { return header[i][0] < header[j][0] })
	var b strings.Builder
	b.WriteString("OAuth ")
	for i, p := range header {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(oauthEncode(p[0]) + `="` + oauthEncode(p[1]) + `"`)
	}
	return b.String(), nil
}

// oauth1Sign computes the OAuth 1.0a HMAC-SHA1 signature (RFC 5849 §3.4) from
// the request method, base URI, and the full set of (decoded) signature
// parameters — oauth_* (minus the signature), query, and form-body pairs.
func oauth1Sign(method, baseURL string, params [][2]string, consumerSecret, tokenSecret string) string {
	enc := make([][2]string, len(params))
	for i, p := range params {
		enc[i] = [2]string{oauthEncode(p[0]), oauthEncode(p[1])}
	}
	sort.Slice(enc, func(i, j int) bool {
		if enc[i][0] != enc[j][0] {
			return enc[i][0] < enc[j][0]
		}
		return enc[i][1] < enc[j][1]
	})
	parts := make([]string, len(enc))
	for i, p := range enc {
		parts[i] = p[0] + "=" + p[1]
	}
	base := strings.ToUpper(method) + "&" + oauthEncode(baseURL) + "&" + oauthEncode(strings.Join(parts, "&"))
	key := oauthEncode(consumerSecret) + "&" + oauthEncode(tokenSecret)
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// oauth1BaseURL returns the signature base URI: lowercased scheme + host
// (default ports dropped) + path, with no query or fragment.
func oauth1BaseURL(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if i := strings.LastIndex(host, ":"); i >= 0 {
		port := host[i+1:]
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			host = host[:i]
		}
	}
	return scheme + "://" + host + u.Path
}

// oauth1FormBody returns the decoded form-body parameters when the request body
// is application/x-www-form-urlencoded (read non-destructively via GetBody),
// else nil. Other body types are not signed (per the common OAuth1 convention).
func oauth1FormBody(req *http.Request) ([][2]string, error) {
	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(ct), "application/x-www-form-urlencoded") || req.GetBody == nil {
		return nil, nil
	}
	rc, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("oauth1: read body: %w", err)
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("oauth1: read body: %w", err)
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, nil // a non-parseable body just isn't signed
	}
	var out [][2]string
	for k, vs := range values {
		for _, v := range vs {
			out = append(out, [2]string{k, v})
		}
	}
	return out, nil
}

// oauthEncode percent-encodes per RFC 3986 / RFC 5849 §3.6: every byte except
// the unreserved set [A-Za-z0-9-._~] becomes %XX (uppercase hex).
func oauthEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
