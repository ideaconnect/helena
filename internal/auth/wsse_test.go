package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestWSSEDigestKnownVector pins #79: PasswordDigest = Base64(SHA1(nonce +
// created + password)). The expected value was computed independently (Python
// hashlib) for these WS-Security UsernameToken inputs, so this is a true
// known-answer test of the spec formula, not a self-referential one.
func TestWSSEDigestKnownVector(t *testing.T) {
	nonce, err := base64.StdEncoding.DecodeString("WScqanjCEAC4mQoBE07sAQ==")
	if err != nil {
		t.Fatal(err)
	}
	got := wsseDigest(nonce, "2003-12-15T14:43:07Z", "taadtaadpstcsm")
	want := "ZdInnzpubrHhbFOJclfWIOtRt1Q="
	if got != want {
		t.Errorf("wsseDigest = %q, want %q", got, want)
	}
}

var wsseField = regexp.MustCompile(`(\w+)="([^"]*)"`)

// TestApplyWSSE verifies Apply emits a well-formed X-WSSE header whose
// PasswordDigest recomputes from its own Nonce + Created, sets the companion
// Authorization profile, and resolves {{vars}} via ResolveValues.
func TestApplyWSSE(t *testing.T) {
	a := model.Auth{Type: model.AuthWSSE, WSSE: &model.WSSEAuth{Username: "{{u}}", Password: "{{p}}"}}
	a = ResolveValues(a, func(s string) string {
		return map[string]string{"{{u}}": "bob", "{{p}}": "s3cret"}[s]
	})

	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	h := req.Header.Get("X-WSSE")
	if !strings.HasPrefix(h, "UsernameToken ") {
		t.Fatalf("X-WSSE = %q", h)
	}
	fields := map[string]string{}
	for _, m := range wsseField.FindAllStringSubmatch(h, -1) {
		fields[m[1]] = m[2]
	}
	if fields["Username"] != "bob" {
		t.Errorf("Username = %q, want bob", fields["Username"])
	}
	nonce, err := base64.StdEncoding.DecodeString(fields["Nonce"])
	if err != nil {
		t.Fatalf("Nonce not base64: %v", err)
	}
	if got := wsseDigest(nonce, fields["Created"], "s3cret"); got != fields["PasswordDigest"] {
		t.Errorf("PasswordDigest %q does not recompute (%q) from its own Nonce/Created", fields["PasswordDigest"], got)
	}
	if req.Header.Get("Authorization") != `WSSE profile="UsernameToken"` {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
}

// TestApplyWSSEDoesNotOverwrite verifies a user-set X-WSSE header is preserved.
func TestApplyWSSEDoesNotOverwrite(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	req.Header.Set("X-WSSE", "preset")
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthWSSE, WSSE: &model.WSSEAuth{Username: "bob", Password: "x"}}, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-WSSE") != "preset" {
		t.Errorf("X-WSSE was overwritten: %q", req.Header.Get("X-WSSE"))
	}
}

// TestApplyWSSENil verifies a WSSE type with no sub-struct is a no-op.
func TestApplyWSSENil(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthWSSE}, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-WSSE") != "" {
		t.Error("nil WSSE should not emit a header")
	}
}
