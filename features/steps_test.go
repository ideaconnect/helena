package features

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cucumber/godog"

	"github.com/idct/helena/internal/model"
)

// InitializeScenario wires the per-scenario world up and registers
// every step the suite uses. Each Before hook constructs a fresh
// world; After cleans up the temp dir + the per-scenario httptest
// server so nothing leaks across scenarios.
func InitializeScenario(sc *godog.ScenarioContext) {
	var w *world

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		nw, err := newWorld()
		if err != nil {
			return ctx, err
		}
		w = nw
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w != nil {
			w.close()
		}
		w = nil
		return ctx, nil
	})

	// Given steps — world setup.
	sc.Step(`^a collection with a request "([^"]*)" GET to the test server$`,
		func(name string) error { return givenRequestToTestServer(w, name, model.GET) })
	sc.Step(`^a collection with a request "([^"]*)" GET to "([^"]*)"$`,
		func(name, url string) error { return givenRequestToURL(w, name, model.GET, url) })
	sc.Step(`^the test server responds with (\d+) "([^"]*)" on "([^"]*)"$`,
		func(status int, body, path string) error {
			w.mux.setHandler(path, func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(status)
				_, _ = rw.Write([]byte(body))
			})
			return nil
		})
	sc.Step(`^the request "([^"]*)" has a pre-script that rewrites the URL to "([^"]*)"$`,
		func(name, url string) error {
			return givenScript(w, name, "pre", fmt.Sprintf("request.url = %q;", url))
		})
	sc.Step(`^the request "([^"]*)" has a pre-script that throws "([^"]*)"$`,
		func(name, msg string) error {
			return givenScript(w, name, "pre", fmt.Sprintf("throw new Error(%q);", msg))
		})
	sc.Step(`^the request "([^"]*)" has a post-script that throws "([^"]*)"$`,
		func(name, msg string) error {
			return givenScript(w, name, "post", fmt.Sprintf("throw new Error(%q);", msg))
		})

	// When step — drives Send.
	sc.Step(`^I send "([^"]*)"$`, func(name string) error {
		if w.sess == nil {
			return fmt.Errorf("no collection seeded")
		}
		// Ensure the base var points at the test server so URL
		// templates referenced in scenarios resolve.
		w.sess.SetEnvOverlay("base", w.server.URL)
		w.send(name)
		return nil
	})

	// Then steps — assertions.
	sc.Step(`^the response status is (\d+)$`, func(want int) error {
		if w.resp == nil {
			return fmt.Errorf("no response (lastErr=%v)", w.lastErr)
		}
		if w.resp.StatusCode != want {
			return fmt.Errorf("status = %d, want %d", w.resp.StatusCode, want)
		}
		return nil
	})
	sc.Step(`^the response body contains "([^"]*)"$`, func(needle string) error {
		if w.resp == nil {
			return fmt.Errorf("no response (lastErr=%v)", w.lastErr)
		}
		if !strings.Contains(string(w.resp.Body), needle) {
			return fmt.Errorf("body = %q, want substring %q", w.resp.Body, needle)
		}
		return nil
	})
	sc.Step(`^sending fails$`, func() error {
		if w.lastErr == nil && w.resp != nil && w.resp.StatusCode != 0 {
			return fmt.Errorf("Send succeeded; want failure (status=%d)", w.resp.StatusCode)
		}
		if w.lastErr == nil {
			return fmt.Errorf("Send produced no error and no response")
		}
		return nil
	})
	sc.Step(`^the last send recorded an error containing "([^"]*)"$`, func(sub string) error {
		if w.lastErr == nil {
			return fmt.Errorf("no error recorded")
		}
		if !strings.Contains(w.lastErr.Error(), sub) {
			return fmt.Errorf("err = %q, want substring %q", w.lastErr, sub)
		}
		return nil
	})
	sc.Step(`^the test server received no requests$`, func() error {
		if n := w.mux.hitCount(); n != 0 {
			return fmt.Errorf("server hit %d times, want 0", n)
		}
		return nil
	})

	// Register chain-feature steps against the same world pointer so
	// Given/When/Then for chain scenarios share scenario state.
	initChainSteps(sc, &w)
	initPersistenceSteps(sc, &w)
	initImportExportSteps(sc, &w)
	initAuthSteps(sc, &w)
}

// givenRequestToTestServer seeds a collection with one request hitting
// the per-scenario test server's root, using the {{base}} overlay
// variable the When step sets.
func givenRequestToTestServer(w *world, name string, method model.Method) error {
	return w.seedCollection(model.Collection{
		Name: "Feature",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: name, Method: method, URL: "{{base}}/",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
		}},
	})
}

// givenRequestToURL is the explicit-URL variant — used by scenarios
// that test the network-error path (URL has no {{base}}).
func givenRequestToURL(w *world, name string, method model.Method, url string) error {
	return w.seedCollection(model.Collection{
		Name: "Feature",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: name, Method: method, URL: url,
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
		}},
	})
}

// givenScript edits the active collection's request in place to add
// the supplied source code as either the pre or post hook. Saves the
// change back to disk so the next session.Load picks it up too
// (mirrors what saveRequest does in the UI).
func givenScript(w *world, name, phase, src string) error {
	if w.sess == nil {
		return fmt.Errorf("no collection seeded")
	}
	r, ok := w.sess.FindRequestByPath(name)
	if !ok {
		return fmt.Errorf("request %q not found", name)
	}
	// FindRequestByPath returns a copy. Mutate the live request in
	// the active collection's slice instead.
	for i := range w.sess.Collections() {
		c := &w.sess.Collections()[i]
		for j := range c.Requests {
			if c.Requests[j].Name == r.Name {
				switch phase {
				case "pre":
					c.Requests[j].Scripts.PreRequest = src
				case "post":
					c.Requests[j].Scripts.PostResponse = src
				}
				return w.sess.SaveActiveCollection()
			}
		}
	}
	return fmt.Errorf("could not locate %q for script update", name)
}
