// Package auth resolves and applies authentication on outgoing requests.
//
// Resolution walks the folder → collection ancestor chain for any request
// whose own Auth is Inherit, picking the nearest non-Inherit value.
// ResolveValues substitutes `{{vars}}` inside the credential fields so a
// Bearer token of `{{TOKEN}}` is replaced before the header is set. Apply
// mutates the outgoing *http.Request — adding an Authorization header for
// Basic / Bearer, or a header / query parameter for API keys.
//
// OAuth2 is recognised (the package returns an error indicating it is not
// yet wired) but the token-fetch flow itself lives in a sibling file that
// will land with task 7.1c.
package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/idct/helena/internal/model"
)

// ErrOAuth2NotImplemented is returned by Apply when an OAuth2 auth is
// configured. Removal of this error follows the OAuth2 grant work in 7.1c.
var ErrOAuth2NotImplemented = errors.New("oauth2 auth is not yet implemented")

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
	default:
		return fmt.Errorf("auth.Apply: unknown auth type %q", a.Type)
	}
}
