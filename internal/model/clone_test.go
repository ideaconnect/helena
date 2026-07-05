package model

import "testing"

// TestAuthCloneDetachesEverySubStruct pins that Auth.Clone gives back a value
// sharing no scheme sub-struct allocation with the original, for all nine
// schemes — so a clone handed to an off-UI worker can't be raced by an in-place
// edit of the live Auth on the UI goroutine.
func TestAuthCloneDetachesEverySubStruct(t *testing.T) {
	orig := Auth{
		Type:   AuthBasic,
		Basic:  &BasicAuth{Password: "b"},
		Bearer: &BearerAuth{Token: "t"},
		APIKey: &APIKeyAuth{Value: "k"},
		OAuth2: &OAuth2Auth{ClientSecret: "o2"},
		WSSE:   &WSSEAuth{Password: "w"},
		OAuth1: &OAuth1Auth{ConsumerSecret: "o1"},
		AWSV4:  &AWSV4Auth{SecretAccessKey: "aws"},
		Digest: &DigestAuth{Password: "d"},
		NTLM:   &NTLMAuth{Password: "n"},
	}
	c := orig.Clone()

	// Distinct allocations.
	if c.Basic == orig.Basic || c.Bearer == orig.Bearer || c.APIKey == orig.APIKey ||
		c.OAuth2 == orig.OAuth2 || c.WSSE == orig.WSSE || c.OAuth1 == orig.OAuth1 ||
		c.AWSV4 == orig.AWSV4 || c.Digest == orig.Digest || c.NTLM == orig.NTLM {
		t.Fatal("Clone shares a sub-struct pointer with the original")
	}

	// Mutating the original's pointees must not bleed into the clone.
	orig.Basic.Password = "X"
	orig.Bearer.Token = "X"
	orig.APIKey.Value = "X"
	orig.OAuth2.ClientSecret = "X"
	orig.WSSE.Password = "X"
	orig.OAuth1.ConsumerSecret = "X"
	orig.AWSV4.SecretAccessKey = "X"
	orig.Digest.Password = "X"
	orig.NTLM.Password = "X"
	if c.Basic.Password != "b" || c.Bearer.Token != "t" || c.APIKey.Value != "k" ||
		c.OAuth2.ClientSecret != "o2" || c.WSSE.Password != "w" || c.OAuth1.ConsumerSecret != "o1" ||
		c.AWSV4.SecretAccessKey != "aws" || c.Digest.Password != "d" || c.NTLM.Password != "n" {
		t.Errorf("Clone shares state with the original: %+v", c)
	}
}

// TestAuthCloneKeepsNilSubStructsNil verifies Clone doesn't materialize nil
// scheme pointers (the common case: only one scheme is set).
func TestAuthCloneKeepsNilSubStructsNil(t *testing.T) {
	c := Auth{Type: AuthBearer, Bearer: &BearerAuth{Token: "t"}}.Clone()
	if c.Basic != nil || c.APIKey != nil || c.OAuth2 != nil || c.WSSE != nil ||
		c.OAuth1 != nil || c.AWSV4 != nil || c.Digest != nil || c.NTLM != nil {
		t.Errorf("Clone materialized a nil sub-struct: %+v", c)
	}
	if c.Bearer == nil || c.Bearer.Token != "t" {
		t.Errorf("Clone dropped the set scheme: %+v", c)
	}
}

// TestRequestCloneDetachesSlicesAndAuth pins Request.Clone: every slice-backed
// field and the Auth sub-structs are detached, so in-place edits and append
// growth on the original can't reach the clone.
func TestRequestCloneDetachesSlicesAndAuth(t *testing.T) {
	orig := Request{
		Params:     []KeyValue{{Key: "p", Value: "1"}},
		Headers:    []KeyValue{{Key: "h", Value: "v"}},
		Body:       Body{Form: []KeyValue{{Key: "f", Value: "x"}}},
		Chain:      []ChainStep{{Alias: "a", Request: "Auth/Login"}},
		Assertions: []Assertion{{Source: "status", Op: "eq", Expected: "200"}},
		Variables:  []Variable{{Key: "v", Value: "1"}},
		Auth:       Auth{Type: AuthBasic, Basic: &BasicAuth{Password: "secret"}},
	}
	c := orig.Clone()

	// In-place edits don't bleed.
	orig.Params[0].Value = "M"
	orig.Headers[0].Value = "M"
	orig.Body.Form[0].Value = "M"
	orig.Chain[0].Request = "M"
	orig.Assertions[0].Expected = "M"
	orig.Variables[0].Value = "M"
	orig.Auth.Basic.Password = "M"
	if c.Params[0].Value != "1" || c.Headers[0].Value != "v" || c.Body.Form[0].Value != "x" ||
		c.Chain[0].Request != "Auth/Login" || c.Assertions[0].Expected != "200" ||
		c.Variables[0].Value != "1" || c.Auth.Basic.Password != "secret" {
		t.Errorf("Clone shares state with the original: %+v", c)
	}

	// Append growth doesn't alias.
	orig.Params = append(orig.Params, KeyValue{Key: "b"})
	orig.Chain = append(orig.Chain, ChainStep{Alias: "z"})
	if len(c.Params) != 1 || len(c.Chain) != 1 {
		t.Errorf("Clone grew with the original: params=%d chain=%d", len(c.Params), len(c.Chain))
	}
}
