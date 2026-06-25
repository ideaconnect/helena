package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAuthTabLoadsWSSE verifies a WSSE request selects the WS-Security type and
// populates the username (#79).
func TestAuthTabLoadsWSSE(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthWSSE, WSSE: &model.WSSEAuth{Username: "bob", Password: "p"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authType.Selected; got != "WS-Security" {
		t.Errorf("authType.Selected = %q, want WS-Security", got)
	}
	if got := m.authWSSEUsername.Text; got != "bob" {
		t.Errorf("username = %q, want bob", got)
	}
}

// TestAuthTabWSSEWriteBack verifies typing into the WSSE fields writes back into
// the request's Auth.WSSE struct (lazily allocated).
func TestAuthTabWSSEWriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthWSSE}}
	m.loadRequest(req, "0/r0")

	m.authWSSEUsername.OnChanged("alice")
	m.authWSSEPassword.OnChanged("s3cret")

	if req.Auth.WSSE == nil || req.Auth.WSSE.Username != "alice" || req.Auth.WSSE.Password != "s3cret" {
		t.Errorf("WSSE = %+v, want alice/s3cret", req.Auth.WSSE)
	}
}
