package examples_test

import (
	"os"
	"testing"
)

// TestMain isolates the externalized-secret store (#42) to a temp dir so the
// sample round-trip test never touches the real OS config dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "helena-secrets-examples")
	if err != nil {
		os.Exit(1)
	}
	_ = os.Setenv("HELENA_SECRETS_DIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
