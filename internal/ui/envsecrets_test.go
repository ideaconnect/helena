package ui

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

var envVarsFixture = []model.Variable{
	{Enabled: true, Key: "HOST", Value: "api.test"},
	{Enabled: true, Key: "TOKEN", Value: "s3cret-value", Secret: true},
}

// TestMaskedEnvTextHidesSecretsByDefault verifies the env editor seeds a masked
// representation for Secret vars (no cleartext) unless reveal is set (#43).
func TestMaskedEnvTextHidesSecretsByDefault(t *testing.T) {
	masked := maskedEnvText(envVarsFixture, false)
	if strings.Contains(masked, "s3cret-value") {
		t.Errorf("masked text leaked the secret value:\n%s", masked)
	}
	if !strings.Contains(masked, envSecretMask) {
		t.Errorf("masked text missing the secret placeholder:\n%s", masked)
	}
	if !strings.Contains(masked, "api.test") {
		t.Errorf("masked text dropped a non-secret value:\n%s", masked)
	}

	revealed := maskedEnvText(envVarsFixture, true)
	if !strings.Contains(revealed, "s3cret-value") {
		t.Errorf("revealed text should show the secret value:\n%s", revealed)
	}
}

// TestRestoreEnvSecretsKeepsHiddenValue verifies that saving with the mask left
// in place keeps the stored secret value and the Secret flag, while a changed
// value is taken as the new secret.
func TestRestoreEnvSecretsKeepsHiddenValue(t *testing.T) {
	secretVals := map[string]string{"TOKEN": "s3cret-value"}

	// User edited only the non-secret line; the secret line still shows the mask.
	unrevealed := []model.Variable{
		{Enabled: true, Key: "HOST", Value: "api.prod"},
		{Enabled: true, Key: "TOKEN", Value: envSecretMask},
	}
	got := restoreEnvSecrets(unrevealed, secretVals)
	tok := findVar(t, got, "TOKEN")
	if tok.Value != "s3cret-value" || !tok.Secret {
		t.Errorf("hidden secret not preserved: %+v", tok)
	}
	if findVar(t, got, "HOST").Value != "api.prod" {
		t.Error("non-secret edit was lost")
	}

	// User revealed and changed the secret: the new value wins, flag preserved.
	changed := []model.Variable{
		{Enabled: true, Key: "TOKEN", Value: "rotated-secret"},
	}
	tok = findVar(t, restoreEnvSecrets(changed, secretVals), "TOKEN")
	if tok.Value != "rotated-secret" || !tok.Secret {
		t.Errorf("changed secret not taken / flag lost: %+v", tok)
	}
}

func findVar(t *testing.T, vs []model.Variable, key string) model.Variable {
	t.Helper()
	for _, v := range vs {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("variable %q not found in %+v", key, vs)
	return model.Variable{}
}
