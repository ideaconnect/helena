package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/idct/helena/internal/model"
)

// emptyPayloadHash is hex(SHA256("")) — the canonical hashed payload for a
// bodyless request.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// awsSigV4Header signs req per AWS Signature Version 4 (#76) and returns the
// value for the Authorization header. amzDate is the ISO-8601 basic timestamp
// (e.g. 20150830T123600Z); payloadHash is hex(SHA256(body)). The X-Amz-Date
// header (and X-Amz-Security-Token, when a.SessionToken is set) must already be
// on req so they are folded into the signed-header set. Service/Region fall
// back to sensible defaults when blank.
func awsSigV4Header(req *http.Request, a *model.AWSV4Auth, amzDate, payloadHash string) string {
	region := a.Region
	if region == "" {
		region = "us-east-1"
	}
	service := a.Service
	if service == "" {
		service = "service"
	}
	dateStamp := amzDate[:8] // YYYYMMDD

	canonicalHeaders, signedHeaders := awsCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		awsCanonicalURI(req.URL.EscapedPath()),
		awsCanonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		awsHashHex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := awsSigningKey(a.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(awsHMAC(signingKey, stringToSign))

	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		a.AccessKeyID, scope, signedHeaders, signature)
}

// awsCanonicalHeaders builds the canonical-headers block and the matching
// signed-headers list. It always signs host + every x-amz-* header present, plus
// content-type when set — the minimum set AWS requires plus the ones that affect
// the request, sorted by lowercased name.
func awsCanonicalHeaders(req *http.Request) (canonical, signed string) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	values := map[string]string{"host": host}
	for name, vs := range req.Header {
		ln := strings.ToLower(name)
		if ln == "content-type" || strings.HasPrefix(ln, "x-amz-") {
			values[ln] = strings.Join(vs, ",")
		}
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		b.WriteByte(':')
		b.WriteString(awsTrimAll(values[n]))
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

// awsCanonicalQuery encodes and sorts the query string per SigV4 (RFC 3986
// encoding of both key and value, sorted by encoded key then value).
func awsCanonicalQuery(q map[string][]string) string {
	pairs := make([]string, 0)
	for k, vs := range q {
		ek := awsURIEncode(k, false)
		for _, v := range vs {
			pairs = append(pairs, ek+"="+awsURIEncode(v, false))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// awsCanonicalURI returns "/" for an empty path, otherwise the request path with
// each segment RFC 3986-encoded (the '/' separators preserved).
func awsCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return awsURIEncode(path, true)
}

// awsSigningKey derives the SigV4 signing key:
// HMAC(HMAC(HMAC(HMAC("AWS4"+secret, date), region), service), "aws4_request").
func awsSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := awsHMAC([]byte("AWS4"+secret), dateStamp)
	kRegion := awsHMAC(kDate, region)
	kService := awsHMAC(kRegion, service)
	return awsHMAC(kService, "aws4_request")
}

func awsHMAC(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func awsHashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// awsTrimAll collapses internal runs of whitespace to a single space and trims
// the ends, per the SigV4 header-value normalization rule.
func awsTrimAll(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// awsURIEncode percent-encodes per RFC 3986: unreserved characters
// (A-Z a-z 0-9 - _ . ~) pass through, everything else becomes %XX (uppercase).
// When keepSlash is true, '/' is left intact (for path segments).
func awsURIEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// awsPayloadHash returns hex(SHA256(body)) for req, reading the body via GetBody
// (so the original body stays consumable). A bodyless request hashes the empty
// string.
func awsPayloadHash(req *http.Request) (string, error) {
	if req.Body == nil || req.GetBody == nil {
		return emptyPayloadHash, nil
	}
	rc, err := req.GetBody()
	if err != nil {
		return "", fmt.Errorf("auth.Apply: awsv4 body: %w", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("auth.Apply: awsv4 body: %w", err)
	}
	if len(body) == 0 {
		return emptyPayloadHash, nil
	}
	return awsHashHex(body), nil
}
