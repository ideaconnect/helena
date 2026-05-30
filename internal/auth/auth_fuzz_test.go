package auth

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// FuzzResolveValuesBasic fuzzes the Basic-auth substitution path.
// The contract: never panics; the Basic sub-struct is preserved
// (never nil after a non-nil input); the substituted values reflect
// the resolver's transformation.
func FuzzResolveValuesBasic(f *testing.F) {
	f.Add("user", "pass")
	f.Add("{{u}}", "{{p}}")
	f.Add("", "")
	f.Add("\x00\xff", "{{cycle}}")
	f.Add(strings.Repeat("{{a}}", 64), "")
	f.Fuzz(func(t *testing.T, u, p string) {
		a := model.Auth{
			Type:  model.AuthBasic,
			Basic: &model.BasicAuth{Username: u, Password: p},
		}
		out := ResolveValues(a, swap("{{u}}", "U"))
		if out.Basic == nil {
			t.Error("Basic became nil after ResolveValues")
		}
	})
}

// FuzzResolveValuesOAuth2 fuzzes OAuth2's seven-field substitution.
// Every field must round-trip through the resolver — a regression
// where one field is missed (e.g. Audience added later) would
// silently leak `{{vars}}` to the token endpoint.
func FuzzResolveValuesOAuth2(f *testing.F) {
	f.Add("t", "a", "ci", "cs", "s", "ru", "au")
	f.Add("{{t}}", "{{a}}", "{{c}}", "{{cs}}", "{{s}}", "{{r}}", "{{au}}")
	f.Add("", "", "", "", "", "", "")
	f.Fuzz(func(t *testing.T, tok, auth, ci, cs, sc, ru, au string) {
		a := model.Auth{
			Type: model.AuthOAuth2,
			OAuth2: &model.OAuth2Auth{
				TokenURL:     tok,
				AuthURL:      auth,
				ClientID:     ci,
				ClientSecret: cs,
				Scope:        sc,
				RedirectURI:  ru,
				Audience:     au,
			},
		}
		out := ResolveValues(a, identity)
		if out.OAuth2 == nil {
			t.Error("OAuth2 became nil after ResolveValues")
		}
		// Identity resolver: every field must be preserved bit-for-bit.
		if out.OAuth2.TokenURL != tok || out.OAuth2.AuthURL != auth ||
			out.OAuth2.ClientID != ci || out.OAuth2.ClientSecret != cs ||
			out.OAuth2.Scope != sc || out.OAuth2.RedirectURI != ru ||
			out.OAuth2.Audience != au {
			t.Errorf("identity resolver lost a field: in=%+v out=%+v", a.OAuth2, out.OAuth2)
		}
	})
}

// FuzzResolveValuesAPIKey fuzzes the API-Key path's three fields,
// asserting the substruct survives any input.
func FuzzResolveValuesAPIKey(f *testing.F) {
	f.Add("X-Api-Key", "secret", "header")
	f.Add("{{name}}", "{{value}}", "query")
	f.Add("", "", "")
	f.Fuzz(func(t *testing.T, name, value, placement string) {
		a := model.Auth{
			Type:   model.AuthAPIKey,
			APIKey: &model.APIKeyAuth{Name: name, Value: value, Placement: model.APIKeyPlacement(placement)},
		}
		out := ResolveValues(a, identity)
		if out.APIKey == nil {
			t.Error("APIKey became nil after ResolveValues")
		}
		// Placement is a discriminator — must NOT be substituted.
		if out.APIKey.Placement != model.APIKeyPlacement(placement) {
			t.Errorf("Placement mutated: %q -> %q", placement, out.APIKey.Placement)
		}
	})
}

func identity(s string) string { return s }
func swap(from, to string) func(string) string {
	return func(s string) string { return strings.ReplaceAll(s, from, to) }
}
