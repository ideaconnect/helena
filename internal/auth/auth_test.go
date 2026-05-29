package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// newRequest builds a throwaway *http.Request for Apply tests.
func newRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}

// TestResolveOwnAuthWins verifies that a request whose own Auth is not
// Inherit short-circuits the ancestor walk and returns its own value.
func TestResolveOwnAuthWins(t *testing.T) {
	own := model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "own"}}
	ancestor := model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "a"}}
	got := Resolve(own, []model.Auth{ancestor})
	if got.Type != model.AuthBearer || got.Bearer == nil || got.Bearer.Token != "own" {
		t.Errorf("Resolve = %+v, want own bearer", got)
	}
}

// TestResolveFallsThroughInherit verifies that the first non-Inherit
// entry on the ancestor chain is picked when the request itself inherits.
func TestResolveFallsThroughInherit(t *testing.T) {
	ancestors := []model.Auth{
		{Type: model.AuthInherit},
		{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "from-folder"}},
		{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "from-collection"}},
	}
	got := Resolve(model.Auth{Type: model.AuthInherit}, ancestors)
	if got.Type != model.AuthBearer || got.Bearer.Token != "from-folder" {
		t.Errorf("Resolve = %+v, want bearer from-folder", got)
	}
}

// TestResolveEverythingInheritsFallsToNone verifies that when nothing on
// the chain has a concrete auth, the resolver returns AuthNone rather
// than something ambiguous.
func TestResolveEverythingInheritsFallsToNone(t *testing.T) {
	got := Resolve(model.Auth{Type: model.AuthInherit}, []model.Auth{
		{Type: model.AuthInherit},
		{Type: ""},
	})
	if got.Type != model.AuthNone {
		t.Errorf("Resolve = %+v, want AuthNone", got)
	}
}

// TestResolveValuesSubstitutesVars verifies that ResolveValues runs every
// credential string through the resolver, returning a deep copy so the
// original sub-struct is untouched.
func TestResolveValuesSubstitutesVars(t *testing.T) {
	a := model.Auth{
		Type:   model.AuthBearer,
		Bearer: &model.BearerAuth{Token: "{{TOKEN}}"},
	}
	resolved := ResolveValues(a, func(s string) string {
		return strings.ReplaceAll(s, "{{TOKEN}}", "abc")
	})
	if resolved.Bearer.Token != "abc" {
		t.Errorf("resolved token = %q, want abc", resolved.Bearer.Token)
	}
	if a.Bearer.Token != "{{TOKEN}}" {
		t.Errorf("original mutated: %q", a.Bearer.Token)
	}
}

// TestApplyBasicSetsAuthorizationHeader verifies that Apply with a Basic
// auth populates `Authorization: Basic <base64(user:pass)>`.
func TestApplyBasicSetsAuthorizationHeader(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	a := model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "u", Password: "p"}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

// TestApplyBearerSetsAuthorizationHeader verifies Bearer adds
// `Authorization: Bearer <token>`.
func TestApplyBearerSetsAuthorizationHeader(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "tok"}}, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization = %q", got)
	}
}

// TestApplyAPIKeyHeader verifies the header placement variant of API-Key
// auth puts the value on the named request header.
func TestApplyAPIKeyHeader(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	a := model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{
		Name: "X-API-Key", Value: "secret", Placement: model.APIKeyHeader,
	}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("X-API-Key"); got != "secret" {
		t.Errorf("X-API-Key = %q", got)
	}
}

// TestApplyAPIKeyQuery verifies the query placement variant appends the
// API-Key as a query parameter, preserving existing query keys.
func TestApplyAPIKeyQuery(t *testing.T) {
	req := newRequest(t, "https://example.com/?existing=1")
	a := model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{
		Name: "key", Value: "v", Placement: model.APIKeyQuery,
	}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	q := req.URL.Query()
	if q.Get("existing") != "1" {
		t.Errorf("existing param lost: %s", req.URL.RawQuery)
	}
	if q.Get("key") != "v" {
		t.Errorf("api key not added: %s", req.URL.RawQuery)
	}
}

// TestApplyNoneAndInheritAreNoops verifies that None and Inherit don't
// touch the request — useful so a caller can pass the unresolved Auth
// straight to Apply without special-casing the empty paths.
func TestApplyNoneAndInheritAreNoops(t *testing.T) {
	for _, typ := range []model.AuthType{model.AuthNone, model.AuthInherit, ""} {
		req := newRequest(t, "https://example.com/?x=1")
		if err := Apply(context.Background(), req, model.Auth{Type: typ}, nil); err != nil {
			t.Errorf("Apply(%q): %v", typ, err)
		}
		if req.Header.Get("Authorization") != "" {
			t.Errorf("Apply(%q) set Authorization unexpectedly", typ)
		}
		if req.URL.RawQuery != "x=1" {
			t.Errorf("Apply(%q) mutated query: %q", typ, req.URL.RawQuery)
		}
	}
}

// TestApplyExistingAuthorizationHeaderWins verifies Apply respects a
// user-provided Authorization header instead of overwriting it.
func TestApplyExistingAuthorizationHeaderWins(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	req.Header.Set("Authorization", "Custom xyz")
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "tok"}}, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Custom xyz" {
		t.Errorf("Authorization overwritten to %q", got)
	}
}

// TestResolveValuesAllAuthTypes verifies that ResolveValues runs each
// auth sub-struct's string fields through the resolver, leaves the
// input untouched, and is a no-op for unset sub-structs.
func TestResolveValuesAllAuthTypes(t *testing.T) {
	resolve := func(s string) string { return strings.ReplaceAll(s, "X", "*") }

	// Basic.
	basic := model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "uX", Password: "pX"}}
	out := ResolveValues(basic, resolve)
	if out.Basic.Username != "u*" || out.Basic.Password != "p*" {
		t.Errorf("Basic = %+v", out.Basic)
	}
	if basic.Basic.Username != "uX" {
		t.Errorf("original Basic mutated: %+v", basic.Basic)
	}

	// API-Key.
	apikey := model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "nX", Value: "vX", Placement: model.APIKeyHeader}}
	out = ResolveValues(apikey, resolve)
	if out.APIKey.Name != "n*" || out.APIKey.Value != "v*" {
		t.Errorf("APIKey = %+v", out.APIKey)
	}

	// OAuth2 — every string field must run through the resolver.
	oa := model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{
		TokenURL: "tX", AuthURL: "aX", ClientID: "ciX", ClientSecret: "csX",
		Scope: "sX", RedirectURI: "rX", Audience: "auX",
	}}
	out = ResolveValues(oa, resolve)
	got := out.OAuth2
	want := model.OAuth2Auth{
		TokenURL: "t*", AuthURL: "a*", ClientID: "ci*", ClientSecret: "cs*",
		Scope: "s*", RedirectURI: "r*", Audience: "au*",
	}
	if got.TokenURL != want.TokenURL || got.AuthURL != want.AuthURL ||
		got.ClientID != want.ClientID || got.ClientSecret != want.ClientSecret ||
		got.Scope != want.Scope || got.RedirectURI != want.RedirectURI ||
		got.Audience != want.Audience {
		t.Errorf("OAuth2 = %+v, want %+v", got, want)
	}

	// Nil sub-struct is a no-op.
	noneOut := ResolveValues(model.Auth{Type: model.AuthBasic}, resolve)
	if noneOut.Basic != nil {
		t.Errorf("nil sub-struct must stay nil, got %+v", noneOut.Basic)
	}
}

// TestResolveValuesNilResolveIsIdentity verifies the explicit nil-
// resolve early return: returns the input unchanged.
func TestResolveValuesNilResolveIsIdentity(t *testing.T) {
	in := model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "{{TOK}}"}}
	out := ResolveValues(in, nil)
	if out.Bearer == nil || out.Bearer.Token != "{{TOK}}" {
		t.Errorf("nil resolve mutated input: %+v", out.Bearer)
	}
}

// TestApplyNilRequestReturnsError verifies Apply guards against a nil
// req rather than panicking.
func TestApplyNilRequestReturnsError(t *testing.T) {
	if err := Apply(context.Background(), nil, model.Auth{Type: model.AuthNone}, nil); err == nil {
		t.Error("expected error for nil req, got nil")
	}
}

// TestApplyAPIKeyEmptyNameNoop verifies that an APIKey with no Name is
// dropped silently (the user is mid-edit) rather than emitting an
// empty-named header / query param.
func TestApplyAPIKeyEmptyNameNoop(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	a := model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "", Value: "v", Placement: model.APIKeyHeader}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if req.URL.RawQuery != "" || len(req.Header) != 0 {
		t.Errorf("empty-name APIKey applied something: q=%q h=%v", req.URL.RawQuery, req.Header)
	}
}

// TestApplyAPIKeyUnknownPlacementErrors verifies an unrecognised
// placement surfaces an error rather than silently dropping the
// credential.
func TestApplyAPIKeyUnknownPlacementErrors(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	a := model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "k", Value: "v", Placement: model.APIKeyPlacement("nonsense")}}
	if err := Apply(context.Background(), req, a, nil); err == nil {
		t.Error("expected error for unknown APIKey placement, got nil")
	}
}

// TestApplyOAuth2NilSubstructIsNoop verifies that AuthOAuth2 with no
// OAuth2 sub-struct returns nil — same convention as the other auth
// types — rather than ErrOAuth2NotImplemented.
func TestApplyOAuth2NilSubstructIsNoop(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthOAuth2}, nil); err != nil {
		t.Errorf("nil OAuth2 sub-struct should be a no-op, got %v", err)
	}
}

// TestApplyOAuth2ExistingAuthorizationWins verifies that an already-set
// Authorization header short-circuits the OAuth2 path so the user's
// explicit choice is preserved.
func TestApplyOAuth2ExistingAuthorizationWins(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	req.Header.Set("Authorization", "Bearer user-set")
	resolver := stubResolver{tok: "would-overwrite"}
	a := model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{Grant: model.OAuth2ClientCredentials}}
	if err := Apply(context.Background(), req, a, resolver); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer user-set" {
		t.Errorf("Authorization = %q, want user's value preserved", got)
	}
}

// TestApplyOAuth2ResolverErrorPropagates verifies that a resolver
// returning an error surfaces it from Apply rather than silently
// sending the request without auth.
func TestApplyOAuth2ResolverErrorPropagates(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	want := errors.New("token endpoint down")
	resolver := stubResolver{err: want}
	a := model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{Grant: model.OAuth2ClientCredentials}}
	err := Apply(context.Background(), req, a, resolver)
	if !errors.Is(err, want) {
		t.Errorf("Apply err = %v, want %v", err, want)
	}
}

// TestApplyUnknownTypeErrors verifies an unrecognised Auth.Type
// surfaces an error so a typo in YAML doesn't silently send no auth.
func TestApplyUnknownTypeErrors(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthType("nonsense")}, nil); err == nil {
		t.Error("expected error for unknown Auth.Type")
	}
}

// stubResolver is a minimal OAuth2Resolver for Apply tests.
type stubResolver struct {
	tok string
	err error
}

func (s stubResolver) Token(context.Context, model.OAuth2Auth) (string, error) {
	return s.tok, s.err
}

// TestApplyOAuth2WithoutResolverReturnsNotImplemented verifies that
// Apply with a nil OAuth2Resolver still surfaces the not-implemented
// sentinel — callers (the exporter, tests, headless paths) that don't
// care about OAuth2 stay informed instead of silently sending no auth.
func TestApplyOAuth2WithoutResolverReturnsNotImplemented(t *testing.T) {
	req := newRequest(t, "https://example.com/")
	err := Apply(context.Background(), req, model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{
		Grant: model.OAuth2ClientCredentials, TokenURL: "https://auth/token",
	}}, nil)
	if !errors.Is(err, ErrOAuth2NotImplemented) {
		t.Errorf("Apply(oauth2, nil resolver) err = %v, want ErrOAuth2NotImplemented", err)
	}
}
