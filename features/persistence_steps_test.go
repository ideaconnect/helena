package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/idct/helena/internal/storage"
)

// initPersistenceSteps registers the step vocabulary used by
// persistence.feature: hand-authoring on-disk YAML, capturing IDs
// across session lifetimes, asserting file contents post-Save,
// reopening a session, and probing the env overlay.
func initPersistenceSteps(sc *godog.ScenarioContext, wp **world) {
	get := func() *world { return *wp }

	sc.Step(`^a hand-authored collection file at "([^"]*)" with content:$`,
		func(rel string, body *godog.DocString) error {
			w := get()
			if err := os.MkdirAll(w.collDir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(w.collDir, rel), []byte(body.Content), 0o644)
		})

	sc.Step(`^I open the collection$`, func() error {
		w := get()
		if w.sess != nil {
			// Already open via an earlier incremental Given — nothing
			// to do; the scenarios that hand-author files always run
			// this step before any incremental Given so this branch
			// shouldn't normally fire.
			return nil
		}
		c, err := storage.Load(w.collDir)
		if err != nil {
			return err
		}
		return w.seedCollection(c)
	})

	sc.Step(`^I save the active collection$`, func() error {
		w := get()
		if w.sess == nil {
			return fmt.Errorf("no active session to save")
		}
		return w.sess.SaveActiveCollection()
	})

	sc.Step(`^I reopen the session$`, func() error { return get().reopen() })

	sc.Step(`^the file at "([^"]*)" contains "([^"]*)"$`,
		func(rel, needle string) error {
			w := get()
			b, err := os.ReadFile(filepath.Join(w.collDir, rel))
			if err != nil {
				return err
			}
			if !strings.Contains(string(b), needle) {
				return fmt.Errorf("%q missing in %s:\n%s", needle, rel, b)
			}
			return nil
		})

	sc.Step(`^the file for "([^"]*)" contains "([^"]*)"$`,
		func(reqPath, needle string) error {
			w := get()
			file := requestFile(w.collDir, reqPath)
			b, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			if !strings.Contains(string(b), needle) {
				return fmt.Errorf("%q missing in %s:\n%s", needle, file, b)
			}
			return nil
		})

	sc.Step(`^I capture the ID of "([^"]*)"$`, func(reqPath string) error {
		w := get()
		r := w.liveRequest(reqPath)
		if r == nil {
			return fmt.Errorf("capture: request %q not found", reqPath)
		}
		w.capturedID = r.ID
		if w.capturedID == "" {
			return fmt.Errorf("capture: request %q has empty ID", reqPath)
		}
		return nil
	})

	sc.Step(`^"([^"]*)" still has the captured ID$`, func(reqPath string) error {
		w := get()
		r := w.liveRequest(reqPath)
		if r == nil {
			return fmt.Errorf("post-reopen: request %q not found", reqPath)
		}
		if r.ID != w.capturedID {
			return fmt.Errorf("ID = %q, want captured %q", r.ID, w.capturedID)
		}
		return nil
	})

	sc.Step(`^the session env overlay sets "([^"]*)" to "([^"]*)"$`,
		func(name, value string) error {
			w := get()
			if err := w.ensureCollection(); err != nil {
				return err
			}
			w.sess.SetEnvOverlay(name, value)
			return nil
		})

	sc.Step(`^the env overlay does not contain "([^"]*)"$`, func(name string) error {
		w := get()
		if w.sess == nil {
			return fmt.Errorf("no session")
		}
		if v, ok := w.sess.EnvOverlay(name); ok {
			return fmt.Errorf("overlay still contains %q = %q", name, v)
		}
		return nil
	})
}

// requestFile maps a chain-ref path like "Auth/Login" to its on-disk
// filename. Mirrors the storage Save naming: lowercase + non-alnum
// collapsed to "-". Used only for asserting "the file for X contains
// Y" inside persistence scenarios.
func requestFile(collDir, reqPath string) string {
	parts := splitPath(reqPath)
	out := collDir
	for i, p := range parts {
		seg := slugify(p)
		if i == len(parts)-1 {
			return filepath.Join(out, seg+".yml")
		}
		out = filepath.Join(out, seg)
	}
	return out
}

// slugify mirrors the storage package's slug behavior for the names
// used in this suite (lowercase ASCII + dash). It's intentionally a
// simplified copy — the suite only ever uses single-word names like
// "Login" / "Profile" / "Auth" / "Authentication" so the round-trip
// behavior is deterministic enough for assertion paths.
func slugify(s string) string {
	var b strings.Builder
	prev := byte('-')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 32)
			prev = c + 32
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteByte(c)
			prev = c
		default:
			if prev != '-' {
				b.WriteByte('-')
				prev = '-'
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
