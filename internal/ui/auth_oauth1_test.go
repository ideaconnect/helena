package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAuthTabLoadsOAuth1 verifies an OAuth1 request selects the type and
// populates the consumer key (#77).
func TestAuthTabLoadsOAuth1(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthOAuth1, OAuth1: &model.OAuth1Auth{ConsumerKey: "ck", Token: "tok"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authType.Selected; got != "OAuth 1.0a" {
		t.Errorf("authType.Selected = %q, want OAuth 1.0a", got)
	}
	if got := m.authOAuth1ConsumerKey.Text; got != "ck" {
		t.Errorf("consumer key = %q, want ck", got)
	}
}

// TestAuthTabOAuth1WriteBack verifies typing into the OAuth1 fields writes back
// into the request's Auth.OAuth1 struct (lazily allocated).
func TestAuthTabOAuth1WriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthOAuth1}}
	m.loadRequest(req, "0/r0")

	m.authOAuth1ConsumerKey.OnChanged("ck")
	m.authOAuth1ConsumerSecret.OnChanged("cs")
	m.authOAuth1Token.OnChanged("tk")
	m.authOAuth1TokenSecret.OnChanged("ts")

	o := req.Auth.OAuth1
	if o == nil || o.ConsumerKey != "ck" || o.ConsumerSecret != "cs" || o.Token != "tk" || o.TokenSecret != "ts" {
		t.Errorf("OAuth1 = %+v, want ck/cs/tk/ts", o)
	}
}
