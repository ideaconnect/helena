package storage

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestScrubRequestSecrets verifies every credential value is blanked while
// non-secret fields survive, and the caller's request is not mutated (#65).
func TestScrubRequestSecrets(t *testing.T) {
	orig := model.Request{
		Method: model.GET,
		URL:    "https://api/secure",
		Auth: model.Auth{
			Type:   model.AuthBasic,
			Basic:  &model.BasicAuth{Username: "alice", Password: "hunter2"},
			Bearer: &model.BearerAuth{Token: "tok-123"},
			APIKey: &model.APIKeyAuth{Name: "X-Key", Value: "key-secret"},
		},
		Variables: []model.Variable{
			{Key: "pub", Value: "shown"},
			{Key: "tok", Value: "hidden", Secret: true},
		},
	}

	got := ScrubRequestSecrets(orig)

	// Secrets blanked.
	if got.Auth.Basic.Password != "" {
		t.Errorf("basic password not scrubbed: %q", got.Auth.Basic.Password)
	}
	if got.Auth.Bearer.Token != "" {
		t.Errorf("bearer token not scrubbed: %q", got.Auth.Bearer.Token)
	}
	if got.Auth.APIKey.Value != "" {
		t.Errorf("api-key value not scrubbed: %q", got.Auth.APIKey.Value)
	}
	// Non-secret auth + request fields preserved.
	if got.Auth.Basic.Username != "alice" || got.Auth.APIKey.Name != "X-Key" {
		t.Errorf("non-secret auth fields lost: %+v", got.Auth)
	}
	if got.URL != "https://api/secure" || got.Method != model.GET {
		t.Errorf("non-secret request fields changed: %s %s", got.Method, got.URL)
	}
	// Variables: secret blanked, non-secret kept.
	for _, v := range got.Variables {
		if v.Secret && v.Value != "" {
			t.Errorf("secret var %q leaked: %q", v.Key, v.Value)
		}
		if !v.Secret && v.Value != "shown" {
			t.Errorf("non-secret var %q changed: %q", v.Key, v.Value)
		}
	}

	// Caller's request untouched (scrub deep-copied auth + vars).
	if orig.Auth.Basic.Password != "hunter2" || orig.Auth.Bearer.Token != "tok-123" ||
		orig.Auth.APIKey.Value != "key-secret" || orig.Variables[1].Value != "hidden" {
		t.Errorf("scrub mutated the caller's request")
	}
}
