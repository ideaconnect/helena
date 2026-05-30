package features

import (
	"fmt"

	"github.com/cucumber/godog"

	"github.com/idct/helena/internal/model"
)

// initAuthSteps registers the step vocabulary used by auth.feature:
// per-auth-type setters on a request, manual header override, and
// per-path hit-count assertions for OAuth2 caching.
func initAuthSteps(sc *godog.ScenarioContext, wp **world) {
	get := func() *world { return *wp }

	sc.Step(`^the request "([^"]*)" has Bearer auth with "([^"]*)"$`,
		func(reqPath, token string) error {
			return setAuth(get(), reqPath, model.Auth{
				Type:   model.AuthBearer,
				Bearer: &model.BearerAuth{Token: token},
			})
		})

	sc.Step(`^the request "([^"]*)" has Basic auth with username "([^"]*)" password "([^"]*)"$`,
		func(reqPath, user, pass string) error {
			return setAuth(get(), reqPath, model.Auth{
				Type:  model.AuthBasic,
				Basic: &model.BasicAuth{Username: user, Password: pass},
			})
		})

	sc.Step(`^the request "([^"]*)" has API-Key auth with name "([^"]*)" value "([^"]*)" in header$`,
		func(reqPath, name, value string) error {
			return setAuth(get(), reqPath, model.Auth{
				Type:   model.AuthAPIKey,
				APIKey: &model.APIKeyAuth{Name: name, Value: value, Placement: model.APIKeyHeader},
			})
		})

	sc.Step(`^the request "([^"]*)" has OAuth2 client_credentials with token URL "([^"]*)" client "([^"]*)" secret "([^"]*)"$`,
		func(reqPath, tokenURL, clientID, secret string) error {
			return setAuth(get(), reqPath, model.Auth{
				Type: model.AuthOAuth2,
				OAuth2: &model.OAuth2Auth{
					Grant:        model.OAuth2ClientCredentials,
					TokenURL:     tokenURL,
					ClientID:     clientID,
					ClientSecret: secret,
				},
			})
		})

	sc.Step(`^the request "([^"]*)" has a header "([^"]*): ([^"]*)"$`,
		func(reqPath, name, value string) error {
			w := get()
			r := w.liveRequest(reqPath)
			if r == nil {
				return fmt.Errorf("set-header: request %q not found", reqPath)
			}
			r.Headers = append(r.Headers, model.KeyValue{Enabled: true, Key: name, Value: value})
			return w.sess.SaveActiveCollection()
		})

	sc.Step(`^the test server received "([^"]*)" exactly (\d+) times?$`,
		func(path string, want int) error {
			got := len(get().mux.observations(path))
			if got != want {
				return fmt.Errorf("%q hit %d times, want %d", path, got, want)
			}
			return nil
		})
}

// setAuth replaces the live request's Auth in place and persists.
func setAuth(w *world, reqPath string, a model.Auth) error {
	r := w.liveRequest(reqPath)
	if r == nil {
		return fmt.Errorf("setAuth: request %q not found", reqPath)
	}
	r.Auth = a
	return w.sess.SaveActiveCollection()
}
