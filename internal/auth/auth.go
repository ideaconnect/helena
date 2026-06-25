// Package auth resolves and applies authentication on outgoing requests.
//
// Resolution walks the folder → collection ancestor chain for any request
// whose own Auth is Inherit, picking the nearest non-Inherit value.
// ResolveValues substitutes `{{vars}}` inside the credential fields so a
// Bearer token of `{{TOKEN}}` is replaced before the header is set. Apply
// mutates the outgoing *http.Request — adding an Authorization header for
// Basic / Bearer, or a header / query parameter for API keys.
//
// OAuth2 is fully wired (client_credentials + authorization_code, with PKCE
// and a token cache) in oauth2.go / oauth2_authcode.go; an unsupported grant
// (or a nil resolver) surfaces as ErrOAuth2NotImplemented.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/idct/helena/internal/model"
)

// ErrOAuth2NotImplemented is returned when an OAuth2 auth can't be honoured:
// no resolver was supplied, or the configured grant isn't supported (only
// client_credentials and authorization_code are). The name is kept for callers
// that match it with errors.Is.
var ErrOAuth2NotImplemented = errors.New("oauth2 grant not supported")

// Resolve picks the effective auth for a request. If reqAuth is anything
// other than Inherit it wins outright. Otherwise the ancestor chain is
// scanned in order (innermost first) and the first non-Inherit entry is
// returned. When every level inherits — or the chain is empty — the
// fallback is AuthNone.
func Resolve(reqAuth model.Auth, ancestors []model.Auth) model.Auth {
	if reqAuth.Type != model.AuthInherit && reqAuth.Type != "" {
		return reqAuth
	}
	for _, a := range ancestors {
		if a.Type != model.AuthInherit && a.Type != "" {
			return a
		}
	}
	return model.Auth{Type: model.AuthNone}
}

// ResolveValues returns a copy of a with every string field run through
// resolve. Only the sub-struct matching a.Type is touched; the rest stay
// untouched (and are typically nil for a well-formed Auth anyway). The
// input is not mutated.
func ResolveValues(a model.Auth, resolve func(string) string) model.Auth {
	if resolve == nil {
		return a
	}
	out := a
	switch a.Type {
	case model.AuthBasic:
		if a.Basic != nil {
			cp := *a.Basic
			cp.Username = resolve(cp.Username)
			cp.Password = resolve(cp.Password)
			out.Basic = &cp
		}
	case model.AuthBearer:
		if a.Bearer != nil {
			cp := *a.Bearer
			cp.Token = resolve(cp.Token)
			out.Bearer = &cp
		}
	case model.AuthAPIKey:
		if a.APIKey != nil {
			cp := *a.APIKey
			cp.Name = resolve(cp.Name)
			cp.Value = resolve(cp.Value)
			out.APIKey = &cp
		}
	case model.AuthOAuth2:
		if a.OAuth2 != nil {
			cp := *a.OAuth2
			cp.TokenURL = resolve(cp.TokenURL)
			cp.AuthURL = resolve(cp.AuthURL)
			cp.ClientID = resolve(cp.ClientID)
			cp.ClientSecret = resolve(cp.ClientSecret)
			cp.Scope = resolve(cp.Scope)
			cp.RedirectURI = resolve(cp.RedirectURI)
			cp.Audience = resolve(cp.Audience)
			out.OAuth2 = &cp
		}
	case model.AuthWSSE:
		if a.WSSE != nil {
			cp := *a.WSSE
			cp.Username = resolve(cp.Username)
			cp.Password = resolve(cp.Password)
			out.WSSE = &cp
		}
	case model.AuthOAuth1:
		if a.OAuth1 != nil {
			cp := *a.OAuth1
			cp.ConsumerKey = resolve(cp.ConsumerKey)
			cp.ConsumerSecret = resolve(cp.ConsumerSecret)
			cp.Token = resolve(cp.Token)
			cp.TokenSecret = resolve(cp.TokenSecret)
			out.OAuth1 = &cp
		}
	case model.AuthAWSV4:
		if a.AWSV4 != nil {
			cp := *a.AWSV4
			cp.AccessKeyID = resolve(cp.AccessKeyID)
			cp.SecretAccessKey = resolve(cp.SecretAccessKey)
			cp.Region = resolve(cp.Region)
			cp.Service = resolve(cp.Service)
			cp.SessionToken = resolve(cp.SessionToken)
			out.AWSV4 = &cp
		}
	case model.AuthDigest:
		if a.Digest != nil {
			cp := *a.Digest
			cp.Username = resolve(cp.Username)
			cp.Password = resolve(cp.Password)
			out.Digest = &cp
		}
	}
	return out
}

// Apply mutates req based on the resolved auth a. Callers are expected to
// have run a through Resolve (to flatten Inherit) and ResolveValues (to
// substitute `{{vars}}`) before reaching here. None and Inherit are
// no-ops. OAuth2 calls resolver.Token to obtain (or refresh) an access
// token and sets it as a Bearer header; a nil resolver — or a resolver
// that doesn't implement the configured grant — surfaces as
// ErrOAuth2NotImplemented. An existing Authorization header set
// explicitly by the user takes precedence: Apply never overwrites it.
func Apply(ctx context.Context, req *http.Request, a model.Auth, resolver OAuth2Resolver) error {
	if req == nil {
		return errors.New("auth.Apply: nil request")
	}
	switch a.Type {
	case "", model.AuthNone, model.AuthInherit:
		return nil
	case model.AuthBasic:
		if a.Basic == nil {
			return nil
		}
		if req.Header.Get("Authorization") != "" {
			return nil
		}
		creds := a.Basic.Username + ":" + a.Basic.Password
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
		return nil
	case model.AuthBearer:
		if a.Bearer == nil {
			return nil
		}
		if req.Header.Get("Authorization") != "" {
			return nil
		}
		req.Header.Set("Authorization", "Bearer "+a.Bearer.Token)
		return nil
	case model.AuthAPIKey:
		if a.APIKey == nil || a.APIKey.Name == "" {
			return nil
		}
		switch a.APIKey.Placement {
		case model.APIKeyQuery:
			// Append rather than round-trip through url.Values{}.Encode(), which
			// would re-sort the whole query and collapse any params that don't
			// round-trip (e.g. repeated keys). This preserves the existing query
			// verbatim and just tacks the key on the end.
			pair := url.QueryEscape(a.APIKey.Name) + "=" + url.QueryEscape(a.APIKey.Value)
			if req.URL.RawQuery == "" {
				req.URL.RawQuery = pair
			} else {
				req.URL.RawQuery += "&" + pair
			}
		case model.APIKeyHeader, "":
			if req.Header.Get(a.APIKey.Name) == "" {
				req.Header.Set(a.APIKey.Name, a.APIKey.Value)
			}
		default:
			return fmt.Errorf("auth.Apply: unknown api-key placement %q", a.APIKey.Placement)
		}
		return nil
	case model.AuthOAuth2:
		if a.OAuth2 == nil {
			return nil
		}
		if resolver == nil {
			return ErrOAuth2NotImplemented
		}
		if req.Header.Get("Authorization") != "" {
			return nil
		}
		token, err := resolver.Token(ctx, *a.OAuth2)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	case model.AuthWSSE:
		if a.WSSE == nil {
			return nil
		}
		if req.Header.Get("X-WSSE") != "" {
			return nil
		}
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("auth.Apply: wsse nonce: %w", err)
		}
		created := time.Now().UTC().Format(time.RFC3339)
		req.Header.Set("X-WSSE", wsseHeader(a.WSSE.Username, a.WSSE.Password, nonce, created))
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", `WSSE profile="UsernameToken"`)
		}
		return nil
	case model.AuthOAuth1:
		if a.OAuth1 == nil {
			return nil
		}
		if req.Header.Get("Authorization") != "" {
			return nil
		}
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("auth.Apply: oauth1 nonce: %w", err)
		}
		header, err := oauth1Header(req, a.OAuth1, hex.EncodeToString(nonce), strconv.FormatInt(time.Now().Unix(), 10))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", header)
		return nil
	case model.AuthAWSV4:
		if a.AWSV4 == nil {
			return nil
		}
		if req.Header.Get("Authorization") != "" {
			return nil
		}
		amzDate := time.Now().UTC().Format("20060102T150405Z")
		req.Header.Set("X-Amz-Date", amzDate)
		if a.AWSV4.SessionToken != "" {
			req.Header.Set("X-Amz-Security-Token", a.AWSV4.SessionToken)
		}
		payloadHash, err := awsPayloadHash(req)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", awsSigV4Header(req, a.AWSV4, amzDate, payloadHash))
		return nil
	case model.AuthDigest:
		// Digest is challenge/response (#75): the first request carries no
		// credentials so the server returns its 401 challenge. The retry with the
		// computed response header is driven by internal/httpclient (DigestRespond).
		// Apply is a deliberate no-op here.
		return nil
	default:
		return fmt.Errorf("auth.Apply: unknown auth type %q", a.Type)
	}
}

// wsseHeader builds the X-WSSE UsernameToken header value (#79). The
// PasswordDigest is Base64(SHA1(nonce + created + password)) per WS-Security,
// where nonce is the raw bytes (the header carries their Base64 form).
func wsseHeader(username, password string, nonce []byte, created string) string {
	digest := wsseDigest(nonce, created, password)
	return fmt.Sprintf(`UsernameToken Username="%s", PasswordDigest="%s", Nonce="%s", Created="%s"`,
		username, digest, base64.StdEncoding.EncodeToString(nonce), created)
}

// wsseDigest computes the WS-Security PasswordDigest:
// Base64(SHA1(nonce || created || password)).
func wsseDigest(nonce []byte, created, password string) string {
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
