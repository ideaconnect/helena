package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestGuardRecoversAndSurfaces verifies a panic inside a guarded callback is
// recovered (no process exit) and surfaced — with no window, via the status
// line (#48). If guard did not recover, this test goroutine would crash.
func TestGuardRecoversAndSurfaces(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess) // no SetWindow: guard falls back to the status line

	ran := false
	m.guard("Import", func() {
		ran = true
		panic("boom-xyz")
	})

	if !ran {
		t.Fatal("guarded fn did not run")
	}
	if got := m.Status.Text; !strings.Contains(got, "Import") || !strings.Contains(got, "boom-xyz") {
		t.Errorf("status did not surface the recovered panic: %q", got)
	}
}

// TestGuardRunsFnNormally verifies guard is transparent on the happy path.
func TestGuardRunsFnNormally(t *testing.T) {
	test.NewApp()
	sess, _ := session.New("")
	m := NewMainUI(sess)

	called := 0
	m.guard("x", func() { called++ })
	if called != 1 {
		t.Errorf("guard called fn %d times, want 1", called)
	}
}
