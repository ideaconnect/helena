package httpclient

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// ntlmTestServer emulates the NTLM-over-HTTP handshake: bare request → 401 NTLM;
// type-1 → 401 with a type-2 challenge; type-3 → 200. It distinguishes type-1
// from type-3 by the NTLMSSP message-type byte. challengeB64 is a synthetic but
// well-formed type-2 CHALLENGE.
func ntlmTestServer(t *testing.T, gotType3 *bool) *httptest.Server {
	t.Helper()
	// A minimal valid CHALLENGE: signature + type 2 + empty target-name + flags +
	// server challenge + reserved + empty target-info (offset 48, len 0).
	ch := make([]byte, 48)
	copy(ch, "NTLMSSP\x00")
	ch[8] = 2
	ch[20] = 0x01 // NEGOTIATE_UNICODE
	copy(ch[24:32], []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef})
	ch[44] = 48 // target-info offset
	challengeB64 := base64.StdEncoding.EncodeToString(ch)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "NTLM ") {
			w.Header().Set("WWW-Authenticate", "NTLM")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		msg, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(authz, "NTLM "))
		if len(msg) < 12 || string(msg[:8]) != "NTLMSSP\x00" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch msg[8] { // message type
		case 1: // NEGOTIATE → return CHALLENGE
			w.Header().Set("WWW-Authenticate", "NTLM "+challengeB64)
			w.WriteHeader(http.StatusUnauthorized)
		case 3: // AUTHENTICATE → success
			*gotType3 = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("welcome"))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestNTLMHandshake drives the full NEGOTIATE → CHALLENGE → AUTHENTICATE flow
// through Do and verifies the request ultimately succeeds (#78).
func TestNTLMHandshake(t *testing.T) {
	var gotType3 bool
	srv := ntlmTestServer(t, &gotType3)

	c := New(model.DefaultSettings())
	req := model.Request{
		Method: model.GET,
		URL:    srv.URL + "/secure",
		Auth:   model.Auth{Type: model.AuthNTLM, NTLM: &model.NTLMAuth{Username: "{{u}}", Password: "Password", Domain: "Domain"}},
	}
	resp, err := c.Do(context.Background(), req, vars.New(map[string]string{"u": "User"}))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !gotType3 {
		t.Error("server never received a type-3 AUTHENTICATE")
	}
	if string(resp.Body) != "welcome" {
		t.Errorf("body = %q, want welcome", resp.Body)
	}
}

// TestNTLMNoOfferNoHandshake verifies a 401 that does not invite NTLM is
// returned untouched (no handshake attempted).
func TestNTLMNoOfferNoHandshake(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("WWW-Authenticate", "Basic realm=x")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(model.DefaultSettings())
	req := model.Request{Method: model.GET, URL: srv.URL,
		Auth: model.Auth{Type: model.AuthNTLM, NTLM: &model.NTLMAuth{Username: "u", Password: "p"}}}
	resp, err := c.Do(context.Background(), req, vars.New(nil))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("server saw %d calls, want 1 (no NTLM offer → no handshake)", calls)
	}
}
