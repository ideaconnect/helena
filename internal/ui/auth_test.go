package ui

import (
	"strings"
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
	if !m.authEd.bearerToken.Password {
		t.Error("Bearer token field is not masked")
	}
	if !m.authEd.apiKeyValue.Password {
		t.Error("API-key value field is not masked")
	}
	if m.authEd.apiKeyName.Password {
		t.Error("API-key name field should not be masked (it is a header/query name)")
	}
	// Sanity: the pre-existing masked fields stayed masked.
	if !m.authEd.basicPassword.Password || !m.authEd.oauth2ClientSecret.Password {
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
	if got := m.authEd.typeSel.Selected; got != "Bearer Token" {
		t.Errorf("authType.Selected = %q, want Bearer Token", got)
	}
	if got := m.authEd.bearerToken.Text; got != "xyz" {
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

	m.authEd.basicUsername.OnChanged("alice")
	m.authEd.basicPassword.OnChanged("hunter2")

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

	m.authEd.apiKeyName.OnChanged("X-Key")
	m.authEd.apiKeyValue.OnChanged("v")
	m.authEd.apiKeyPlacement.OnChanged("Query")
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
	m.authEd.typeSel.OnChanged("Basic Auth")
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

// TestAuthTabInheritPreviewUsesAncestors verifies the Inherit panel previews
// what the request would inherit (Session.InheritedAuth), not the request's
// own saved auth — switching a Bearer request to Inherit must show the
// collection root's scheme, not echo "Bearer" back.
func TestAuthTabInheritPreviewUsesAncestors(t *testing.T) {
	m, _, _ := newSettingsUI(t)
	// Give the folder request its own Basic auth, saved.
	req, _ := m.sess.Tree().Request("0/f0/r0")
	req.Auth = model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "u"}}
	m.openOrActivate("0/f0/r0")
	if m.authEd.typeSel.Selected != "Basic Auth" {
		t.Fatalf("loaded type = %q, want Basic Auth", m.authEd.typeSel.Selected)
	}
	m.authEd.typeSel.SetSelected("Inherit from parent")
	if got := m.authEd.inheritLabel.Text; !strings.Contains(got, "Bearer Token") {
		t.Errorf("inherit preview = %q, want the collection root's Bearer", got)
	}
}

// TestAuthTabNoRequestInheritPreview verifies the scratch / no-request case
// keeps its explanatory line.
func TestAuthTabNoRequestInheritPreview(t *testing.T) {
	m := newAuthUI(t)
	m.loadAuthTab(nil)
	if got := m.authEd.inheritLabel.Text; !strings.Contains(got, "no request selected") {
		t.Errorf("inherit preview with no request = %q", got)
	}
}
