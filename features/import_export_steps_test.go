package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/idct/helena/internal/exporter"
	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
	"github.com/idct/helena/internal/vars"
)

// initImportExportSteps registers the step vocabulary used by
// import_export.feature: import an OpenAPI / Swagger spec into a
// Helena collection, assert the tree shape, and render an imported
// request as cURL.
func initImportExportSteps(sc *godog.ScenarioContext, wp **world) {
	get := func() *world { return *wp }

	sc.Step(`^an OpenAPI 3 spec:$`, func(body *godog.DocString) error {
		get().captureVars["spec"] = body.Content
		return nil
	})
	sc.Step(`^a Swagger 2 spec:$`, func(body *godog.DocString) error {
		get().captureVars["spec"] = body.Content
		return nil
	})

	sc.Step(`^I import the spec$`, func() error {
		w := get()
		spec, ok := w.captureVars["spec"]
		if !ok {
			return fmt.Errorf("import: no spec captured (use 'Given an OpenAPI 3 spec:' first)")
		}
		c, err := importer.FromOpenAPI([]byte(spec))
		if err != nil {
			return fmt.Errorf("import: %w", err)
		}
		if err := storage.Save(c, w.collDir); err != nil {
			return fmt.Errorf("save imported: %w", err)
		}
		return w.reopenWith(w.collDir)
	})

	sc.Step(`^the collection has a folder "([^"]*)"$`, func(name string) error {
		if w := get(); w.liveFolder(name) == nil {
			return fmt.Errorf("folder %q not found", name)
		}
		return nil
	})

	sc.Step(`^the collection has a request "([^"]*)"$`, func(reqPath string) error {
		if w := get(); w.liveRequest(reqPath) == nil {
			return fmt.Errorf("request %q not found", reqPath)
		}
		return nil
	})

	sc.Step(`^the collection has an environment variable "([^"]*)" set to "([^"]*)"$`,
		func(name, want string) error {
			w := get()
			cols := w.sess.Collections()
			if len(cols) == 0 || len(cols[0].Environments) == 0 {
				return fmt.Errorf("no environments loaded")
			}
			for _, e := range cols[0].Environments {
				for _, v := range e.Variables {
					if v.Key == name {
						if v.Value != want {
							return fmt.Errorf("env var %q = %q, want %q", name, v.Value, want)
						}
						return nil
					}
				}
			}
			return fmt.Errorf("env var %q not found", name)
		})

	sc.Step(`^the request "([^"]*)" has method "([^"]*)"$`,
		func(reqPath, want string) error {
			w := get()
			r := w.liveRequest(reqPath)
			if r == nil {
				return fmt.Errorf("request %q not found", reqPath)
			}
			if string(r.Method) != want {
				return fmt.Errorf("method = %q, want %q", r.Method, want)
			}
			return nil
		})

	sc.Step(`^the request "([^"]*)" body contains "([^"]*)"$`,
		func(reqPath, needle string) error {
			w := get()
			r := w.liveRequest(reqPath)
			if r == nil {
				return fmt.Errorf("request %q not found", reqPath)
			}
			if !strings.Contains(r.Body.Content, needle) {
				return fmt.Errorf("body = %q, want substring %q", r.Body.Content, needle)
			}
			return nil
		})

	sc.Step(`^I render "([^"]*)" as cURL$`, func(reqPath string) error {
		w := get()
		r := w.liveRequest(reqPath)
		if r == nil {
			return fmt.Errorf("render: request %q not found", reqPath)
		}
		// Build a resolver from the first environment so {{base_url}}
		// substitutes to the imported server URL.
		envVars := map[string]string{}
		if cols := w.sess.Collections(); len(cols) > 0 && len(cols[0].Environments) > 0 {
			for _, v := range cols[0].Environments[0].Variables {
				if v.Enabled {
					envVars[v.Key] = v.Value
				}
			}
		}
		res := vars.New(envVars)
		out, err := exporter.ToCurl(*r, res, model.DefaultSettings())
		if err != nil {
			return fmt.Errorf("ToCurl: %w", err)
		}
		w.curl = out
		return nil
	})

	sc.Step(`^the cURL contains "([^"]*)"$`, func(needle string) error {
		w := get()
		if w.curl == "" {
			return fmt.Errorf("no cURL rendered (use 'I render X as cURL' first)")
		}
		if !strings.Contains(w.curl, needle) {
			return fmt.Errorf("cURL missing %q in:\n%s", needle, w.curl)
		}
		return nil
	})
}
