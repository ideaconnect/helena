// Package features hosts Helena's behavioural test suite, run via
// godog (Gherkin / Cucumber for Go — the closest equivalent to Behat
// in this ecosystem).
//
// Run with:
//
//	go test ./features/...
//
// Feature files live in this directory; each .feature file pairs with
// step definitions registered in InitializeScenario. World state
// (current session, last response, captured errors) is per-scenario —
// see world.go.
//
// Scope: every step here drives Helena through the internal packages
// (storage, session, httpclient, chain, scripting). The UI is
// deliberately excluded — Phase 11 will revisit it with a separate
// harness if needed.
package features

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures is the Go entrypoint godog runs the suite under.
// godog discovers every .feature file under the working directory
// (./features when invoked from the repo root, or "" when invoked
// from inside the package) and pairs each scenario step with the
// registered handlers in InitializeScenario.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
			Strict:   true,
			Output:   os.Stdout,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("feature suite failed")
	}
}
