package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAuthTabLoadsNTLM verifies an NTLM request selects the type and populates
// the username + domain (#78).
func TestAuthTabLoadsNTLM(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthNTLM, NTLM: &model.NTLMAuth{Username: "alice", Domain: "CORP"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authEd.typeSel.Selected; got != "NTLM" {
		t.Errorf("authType.Selected = %q, want NTLM", got)
	}
	if got := m.authEd.ntlmUsername.Text; got != "alice" {
		t.Errorf("username = %q, want alice", got)
	}
	if got := m.authEd.ntlmDomain.Text; got != "CORP" {
		t.Errorf("domain = %q, want CORP", got)
	}
}

// TestAuthTabNTLMWriteBack verifies typing into the NTLM fields writes back into
// the request's Auth.NTLM struct (lazily allocated).
func TestAuthTabNTLMWriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthNTLM}}
	m.loadRequest(req, "0/r0")

	m.authEd.ntlmUsername.OnChanged("u")
	m.authEd.ntlmPassword.OnChanged("p")
	m.authEd.ntlmDomain.OnChanged("d")
	m.authEd.ntlmWorkstation.OnChanged("ws")

	n := req.Auth.NTLM
	if n == nil || n.Username != "u" || n.Password != "p" || n.Domain != "d" || n.Workstation != "ws" {
		t.Errorf("NTLM = %+v, want u/p/d/ws", n)
	}
}
