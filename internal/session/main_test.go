package session

import (
	"os"
	"testing"
)

// TestMain isolates the externalized-secret store (#42) to a temp dir for the
// whole package so tests never touch the real OS config dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "helena-secrets-session")
	if err != nil {
		os.Exit(1)
	}
	_ = os.Setenv("HELENA_SECRETS_DIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
