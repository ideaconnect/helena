package features

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cucumber/godog"

	"github.com/idct/helena/internal/model"
)

// initChainSteps registers the step definitions used by chain.feature
// (and any future feature that builds requests incrementally). Kept
// separate from steps_test.go to avoid one giant file once the suite
// grows past send + chain.
//
// Convention for new chain steps:
//   - paths in Gherkin start with "/" — the step auto-prefixes
//     {{base}} so URLs resolve to the per-scenario test server.
//   - request paths use the same slash-separated form
//     FindRequestByPath understands (e.g. "Auth/Login").
//   - body assertions go through the handlerMux's observations log.
func initChainSteps(sc *godog.ScenarioContext, wp **world) {
	get := func() *world { return *wp }

	sc.Step(`^a request "([^"]*)" (GET|POST|PUT|DELETE|PATCH) to "([^"]*)"$`,
		func(name, method, path string) error {
			return addRequest(get(), "", name, method, path)
		})
	sc.Step(`^a request "([^"]*)" (GET|POST|PUT|DELETE|PATCH) to "([^"]*)" inside folder "([^"]*)"$`,
		func(name, method, path, folder string) error {
			return addRequest(get(), folder, name, method, path)
		})

	sc.Step(`^a folder "([^"]*)" with Bearer "([^"]*)"$`,
		func(name, token string) error {
			w := get()
			if err := w.ensureCollection(); err != nil {
				return err
			}
			if _, err := w.sess.AddFolder("0", name); err != nil {
				return err
			}
			f := w.liveFolder(name)
			if f == nil {
				return fmt.Errorf("folder %q not found after AddFolder", name)
			}
			f.Auth = model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: token}}
			return w.sess.SaveActiveCollection()
		})

	sc.Step(`^the request "([^"]*)" chains "([^"]*)" to "([^"]*)"$`,
		func(reqPath, alias, target string) error {
			return appendChainStep(get(), reqPath, alias, target, false)
		})
	sc.Step(`^the request "([^"]*)" chains "([^"]*)" to "([^"]*)" with pinned ID$`,
		func(reqPath, alias, target string) error {
			return appendChainStep(get(), reqPath, alias, target, true)
		})

	sc.Step(`^the request "([^"]*)" has a pre-script:$`,
		func(reqPath string, body *godog.DocString) error {
			return setScript(get(), reqPath, "pre", body.Content)
		})
	sc.Step(`^the request "([^"]*)" has a post-script:$`,
		func(reqPath string, body *godog.DocString) error {
			return setScript(get(), reqPath, "post", body.Content)
		})

	sc.Step(`^the test server responds with JSON on "([^"]*)":$`,
		func(path string, body *godog.DocString) error {
			get().mux.setHandler(path, func(rw http.ResponseWriter, _ *http.Request) {
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusOK)
				_, _ = rw.Write([]byte(body.Content))
			})
			return nil
		})

	sc.Step(`^I rename "([^"]*)" to "([^"]*)"$`,
		func(target, newName string) error {
			w := get()
			id := w.nodeIDFor(target)
			if id == "" {
				return fmt.Errorf("rename: node %q not found", target)
			}
			return w.sess.RenameItem(id, newName)
		})

	sc.Step(`^a linear chain of depth (\d+)$`, func(depth int) error {
		w := get()
		if err := w.ensureCollection(); err != nil {
			return err
		}
		// Create R0..R(depth-1). R0 is the leaf; each Ri chains to R(i+1).
		for i := 0; i < depth; i++ {
			name := fmt.Sprintf("R%d", i)
			if err := addRequest(w, "", name, "GET", fmt.Sprintf("/r%d", i)); err != nil {
				return err
			}
		}
		for i := 0; i < depth-1; i++ {
			leaf := fmt.Sprintf("R%d", i)
			pred := fmt.Sprintf("R%d", i+1)
			if err := appendChainStep(w, leaf, "next", pred, false); err != nil {
				return err
			}
		}
		return w.sess.SaveActiveCollection()
	})

	sc.Step(`^the test server received "([^"]*)"$`, func(path string) error {
		if obs := get().mux.observations(path); len(obs) == 0 {
			return fmt.Errorf("test server never hit %q", path)
		}
		return nil
	})
	sc.Step(`^the test server received "([^"]*)" with header "([^"]*): ([^"]*)"$`,
		func(path, h, v string) error {
			obs := get().mux.observations(path)
			if len(obs) == 0 {
				return fmt.Errorf("no observations for %q", path)
			}
			for _, o := range obs {
				if got := o.Headers.Get(h); got == v {
					return nil
				}
			}
			return fmt.Errorf("%q never seen with header %q: %q (observed %d hits)", path, h, v, len(obs))
		})
	sc.Step(`^the test server received "([^"]*)" with query "([^"]*)=([^"]*)"$`,
		func(path, k, v string) error {
			obs := get().mux.observations(path)
			if len(obs) == 0 {
				return fmt.Errorf("no observations for %q", path)
			}
			for _, o := range obs {
				if got := o.Query.Get(k); got == v {
					return nil
				}
			}
			return fmt.Errorf("%q never seen with query %s=%s", path, k, v)
		})
}

// addRequest extends the active collection with a new request at the
// top level (folder == "") or inside an existing folder. URL is
// prefixed with {{base}} so scenarios can write rooted paths and have
// them resolve to the per-scenario test server at Send time.
func addRequest(w *world, folder, name, method, path string) error {
	if err := w.ensureCollection(); err != nil {
		return err
	}
	parent := "0"
	if folder != "" {
		parent = w.nodeIDFor(folder)
		if parent == "" {
			return fmt.Errorf("addRequest: parent folder %q not found", folder)
		}
	}
	if _, err := w.sess.AddRequest(parent, name); err != nil {
		return err
	}
	r := w.liveRequest(joinNonEmpty(folder, name))
	if r == nil {
		return fmt.Errorf("addRequest: %q not found after AddRequest", name)
	}
	r.Method = model.Method(method)
	r.URL = "{{base}}" + path
	r.Auth = model.Auth{Type: model.AuthInherit}
	return w.sess.SaveActiveCollection()
}

// appendChainStep adds a ChainStep to the request at reqPath. When
// pinID is true, the target's current Request.ID is captured as
// ChainStep.RequestID so the ref survives a subsequent rename of the
// target.
func appendChainStep(w *world, reqPath, alias, target string, pinID bool) error {
	r := w.liveRequest(reqPath)
	if r == nil {
		return fmt.Errorf("appendChainStep: request %q not found", reqPath)
	}
	step := model.ChainStep{Alias: alias, Request: target}
	if pinID {
		id, ok := w.sess.RequestIDForPath(target)
		if !ok {
			return fmt.Errorf("appendChainStep: target %q has no ID", target)
		}
		step.RequestID = id
	}
	r.Chain = append(r.Chain, step)
	return w.sess.SaveActiveCollection()
}

// setScript edits the live request at reqPath in place to set the
// pre or post script source. Persists so subsequent Loads see it.
func setScript(w *world, reqPath, phase, src string) error {
	r := w.liveRequest(reqPath)
	if r == nil {
		return fmt.Errorf("setScript: request %q not found", reqPath)
	}
	switch phase {
	case "pre":
		r.Scripts.PreRequest = strings.TrimSpace(src) + "\n"
	case "post":
		r.Scripts.PostResponse = strings.TrimSpace(src) + "\n"
	}
	return w.sess.SaveActiveCollection()
}

// joinNonEmpty joins folder + name into a chain-ref path, dropping the
// folder segment when empty.
func joinNonEmpty(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}
