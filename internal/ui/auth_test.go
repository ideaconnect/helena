package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// newAuthUI builds a MainUI with no real window, ready for Auth-tab tests.
func newAuthUI(t *testing.T) *MainUI {
	t.Helper()
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	m := NewMainUI(sess)
	test.NewWindow(m.Root()) // keep the layout realized
	return m
}

// TestAuthSecretFieldsMasked verifies the credential entries that previously
// rendered in cleartext are masked like the Basic password field (#44): the
// Bearer token and the API-key value. The API-key Name stays visible (it's a
// header name, not a secret).
func TestAuthSecretFieldsMasked(t *testing.T) {
	m := newAuthUI(t)
	if !m.authBearerToken.Password {
		t.Error("Bearer token field is not masked")
	}
	if !m.authAPIKeyValue.Password {
		t.Error("API-key value field is not masked")
	}
	if m.authAPIKeyName.Password {
		t.Error("API-key name field should not be masked (it is a header/query name)")
	}
	// Sanity: the pre-existing masked fields stayed masked.
	if !m.authBasicPassword.Password || !m.authOAuth2ClientSecret.Password {
		t.Error("a previously-masked credential field is no longer masked")
	}
}

// TestAuthTabLoadsBearer verifies that loading a request with Bearer auth
// switches the Type dropdown to "Bearer Token" and populates the token
// entry.
func TestAuthTabLoadsBearer(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "xyz"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authType.Selected; got != "Bearer Token" {
		t.Errorf("authType.Selected = %q, want Bearer Token", got)
	}
	if got := m.authBearerToken.Text; got != "xyz" {
		t.Errorf("token = %q, want xyz", got)
	}
}

// TestAuthTabBasicWriteBack verifies that typing into the Basic Auth
// fields writes back into the in-memory request's Auth.Basic struct
// (lazily allocating it the first time).
func TestAuthTabBasicWriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthBasic}}
	m.loadRequest(req, "0/r0")

	m.authBasicUsername.OnChanged("alice")
	m.authBasicPassword.OnChanged("hunter2")

	if req.Auth.Basic == nil {
		t.Fatal("Basic sub-struct should have been allocated")
	}
	if req.Auth.Basic.Username != "alice" || req.Auth.Basic.Password != "hunter2" {
		t.Errorf("Basic = %+v, want alice/hunter2", req.Auth.Basic)
	}
}

// TestAuthTabAPIKeyPlacement verifies that the placement dropdown writes
// through to model.APIKeyPlacement.
func TestAuthTabAPIKeyPlacement(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthAPIKey}}
	m.loadRequest(req, "0/r0")

	m.authAPIKeyName.OnChanged("X-Key")
	m.authAPIKeyValue.OnChanged("v")
	m.authAPIKeyPlacement.OnChanged("Query")
	if k := req.Auth.APIKey; k == nil || k.Name != "X-Key" || k.Value != "v" || k.Placement != model.APIKeyQuery {
		t.Errorf("APIKey = %+v", k)
	}
}

// TestAuthTabTypeChangeUpdatesRequestType verifies that the Type dropdown
// drives model.Request.Auth.Type so a switch from Inherit to Basic shows
// up in subsequent EffectiveAuth / save flows.
func TestAuthTabTypeChangeUpdatesRequestType(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthInherit}}
	m.loadRequest(req, "0/r0")
	m.authType.OnChanged("Basic Auth")
	if req.Auth.Type != model.AuthBasic {
		t.Errorf("Auth.Type = %q, want basic", req.Auth.Type)
	}
}

// TestAuthTabLoadingFlagSuppressesWriteBack verifies that loadRequest
// itself doesn't accidentally overwrite the request's Auth via the
// OnChanged callbacks — the m.loading guard is doing its job.
func TestAuthTabLoadingFlagSuppressesWriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{
		Type:   model.AuthAPIKey,
		APIKey: &model.APIKeyAuth{Name: "K", Value: "V", Placement: model.APIKeyQuery},
	}}
	m.loadRequest(req, "0/r0")
	// The widgets are showing K/V/Query; nothing in the request should have
	// been allocated again or zeroed by the load.
	if req.Auth.APIKey == nil || req.Auth.APIKey.Name != "K" || req.Auth.APIKey.Value != "V" || req.Auth.APIKey.Placement != model.APIKeyQuery {
		t.Errorf("APIKey perturbed by load: %+v", req.Auth.APIKey)
	}
}
