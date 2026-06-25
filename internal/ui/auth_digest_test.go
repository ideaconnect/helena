package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAuthTabLoadsDigest verifies a Digest request selects the type and
// populates the username (#75).
func TestAuthTabLoadsDigest(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthDigest, Digest: &model.DigestAuth{Username: "mufasa"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authType.Selected; got != "Digest Auth" {
		t.Errorf("authType.Selected = %q, want Digest Auth", got)
	}
	if got := m.authDigestUsername.Text; got != "mufasa" {
		t.Errorf("username = %q, want mufasa", got)
	}
}

// TestAuthTabDigestWriteBack verifies typing into the Digest fields writes back
// into the request's Auth.Digest struct (lazily allocated).
func TestAuthTabDigestWriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthDigest}}
	m.loadRequest(req, "0/r0")

	m.authDigestUsername.OnChanged("u")
	m.authDigestPassword.OnChanged("p")

	d := req.Auth.Digest
	if d == nil || d.Username != "u" || d.Password != "p" {
		t.Errorf("Digest = %+v, want u/p", d)
	}
}
