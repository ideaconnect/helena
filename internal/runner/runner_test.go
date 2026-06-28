package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// openColl saves col to a temp dir and returns an active session over it.
func openColl(t *testing.T, col model.Collection) *session.Session {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "c")
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	s, err := session.New("")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	return s
}

// byPath returns the result for a request path, or nil.
func byPath(rep Report, path string) *RequestResult {
	for i := range rep.Results {
		if rep.Results[i].Path == path {
			return &rep.Results[i]
		}
	}
	return nil
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setcookie" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "xyz", Path: "/"})
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		if r.URL.Path == "/ok" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true,"id":7}`))
			return
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRunMixed exercises the engine end-to-end: assertions (pass + fail), a
// post-response test() script, a folder request, and a 500 response.
func TestRunMixed(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name: "Health", Method: model.GET, URL: srv.URL + "/ok",
			Assertions: []model.Assertion{
				{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"},
				{Enabled: true, Source: "res.json.id", Op: "greaterThan", Expected: "5"},
				{Enabled: true, Source: "res.json.id", Op: "equals", Expected: "999"}, // fails
			},
			Scripts: model.Scripts{PostResponse: `test("ok flag", function(){ expect(response.json.ok).toBeTruthy(); });`},
		}},
		Folders: []model.Folder{{
			Name: "Sub",
			Requests: []model.Request{{
				Name: "Broken", Method: model.GET, URL: srv.URL + "/fail",
				Assertions: []model.Assertion{{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"}}, // fails
			}},
		}},
	}
	rep := Run(context.Background(), openColl(t, col))

	if len(rep.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(rep.Results))
	}
	h := byPath(rep, "Health")
	if h == nil || h.StatusCode != 200 {
		t.Fatalf("health = %+v", h)
	}
	if len(h.Checks) != 4 { // 3 assertions + 1 test()
		t.Errorf("health checks = %d, want 4", len(h.Checks))
	}
	if h.OK() {
		t.Errorf("health should NOT be OK (one assertion fails)")
	}
	b := byPath(rep, "Sub/Broken")
	if b == nil || b.StatusCode != 500 || b.OK() {
		t.Errorf("broken = %+v, want Sub/Broken 500 not-OK", b)
	}
	if !rep.Failed() {
		t.Error("report should be Failed()")
	}
	reqs, passed, failed := rep.Totals()
	if reqs != 2 || passed == 0 || failed == 0 {
		t.Errorf("totals = %d/%d/%d", reqs, passed, failed)
	}
}

// TestRunAllPass verifies a collection whose checks all pass reports success.
func TestRunAllPass(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name: "Health", Method: model.GET, URL: srv.URL + "/ok",
			Assertions: []model.Assertion{
				{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"},
				{Enabled: true, Source: "res.json.ok", Op: "equals", Expected: "true"},
			},
		}},
	}
	rep := Run(context.Background(), openColl(t, col))
	if rep.Failed() {
		t.Errorf("all-pass collection should not Failed(): %+v", rep.Results)
	}
	if !rep.Results[0].OK() {
		t.Errorf("request should be OK: %+v", rep.Results[0])
	}
}

// TestRunRequestError verifies a request that fails to connect is reported with
// an error and counts as a failure.
func TestRunRequestError(t *testing.T) {
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "Dead", Method: model.GET, URL: "http://127.0.0.1:1/nope"}},
	}
	rep := Run(context.Background(), openColl(t, col))
	if len(rep.Results) != 1 || rep.Results[0].Err == "" || rep.Results[0].OK() {
		t.Errorf("expected a request error: %+v", rep.Results)
	}
	if !rep.Failed() {
		t.Error("report with a request error should be Failed()")
	}
}

// TestRunChainScriptsAndError exercises a chain step, env-overlay script
// reads/writes, request variables, and a post-script that throws.
func TestRunChainScriptsAndError(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{
			{Name: "Health", Method: model.GET, URL: srv.URL + "/ok"},
			{
				Name: "Leaf", Method: model.GET, URL: srv.URL + "/ok",
				Chain:     []model.ChainStep{{Alias: "pre", Request: "Health"}},
				Variables: []model.Variable{{Enabled: true, Key: "v", Value: "1"}},
				Scripts: model.Scripts{PostResponse: `
					helena.env.set("x", "1");
					test("env + chain", function () {
						expect(helena.env.get("x")).toBe("1");
						expect(chain.pre.response.status).toBe(200);
						// helena.interpolate resolves the request variable v (#92)
						// and reflects the just-set overlay value x.
						expect(helena.interpolate("{{v}}")).toBe("1");
						expect(helena.interpolate("{{x}}")).toBe("1");
					});`},
			},
			{Name: "Boom", Method: model.GET, URL: srv.URL + "/ok",
				Scripts: model.Scripts{PostResponse: `throw new Error("kaboom");`}},
		},
	}
	rep := Run(context.Background(), openColl(t, col))

	leaf := byPath(rep, "Leaf")
	if leaf == nil || len(leaf.Checks) != 1 || !leaf.Checks[0].Passed {
		t.Errorf("leaf chain/env test = %+v", leaf)
	}
	boom := byPath(rep, "Boom")
	if boom == nil || boom.Err == "" {
		t.Errorf("boom should carry a post-script error: %+v", boom)
	}
}

// TestRunEnvVarResolution verifies the runner resolves {{vars}} from the active
// environment (so a CLI run matches a GUI Send).
func TestRunEnvVarResolution(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name: "Health", Method: model.GET, URL: "{{base}}/ok",
			Assertions: []model.Assertion{{Enabled: true, Source: "res.status", Op: "equals", Expected: "200"}},
		}},
		Environments: []model.Environment{{
			Name:      "Dev",
			Variables: []model.Variable{{Enabled: true, Key: "base", Value: srv.URL}},
		}},
	}
	s := openColl(t, col)
	s.SetActiveEnv("Dev")
	rep := Run(context.Background(), s)
	if rep.Failed() {
		t.Errorf("env var should resolve and request succeed: %+v", rep.Results)
	}
}

// TestRunSendRequestFromScript exercises helena.sendRequest end-to-end (#92): a
// post-response script fires an ad-hoc request through the host client and
// asserts on the returned response object.
func TestRunSendRequestFromScript(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name: "Leaf", Method: model.GET, URL: srv.URL + "/ok",
			Scripts: model.Scripts{PostResponse: `
				var r = helena.sendRequest({ url: "` + srv.URL + `/ok" });
				test("sendRequest", function () {
					expect(r.status).toBe(200);
					expect(r.json.id).toBe(7);
				});`},
		}},
	}
	rep := Run(context.Background(), openColl(t, col))
	leaf := byPath(rep, "Leaf")
	if leaf == nil || len(leaf.Checks) != 1 || !leaf.Checks[0].Passed {
		t.Errorf("sendRequest end-to-end test = %+v", leaf)
	}
}

// TestRunCookiesFromScript exercises helena.cookies end-to-end (#92): a first
// request receives a Set-Cookie that lands in the shared jar, and a later
// request's script reads it back for the server URL.
func TestRunCookiesFromScript(t *testing.T) {
	srv := testServer(t)
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{
			{Name: "SetIt", Method: model.GET, URL: srv.URL + "/setcookie"},
			{
				Name: "Leaf", Method: model.GET, URL: srv.URL + "/ok",
				Scripts: model.Scripts{PostResponse: `
					test("cookie", function () {
						expect(helena.cookies.get("` + srv.URL + `/ok", "session")).toBe("xyz");
					});`},
			},
		},
	}
	rep := Run(context.Background(), openColl(t, col))
	leaf := byPath(rep, "Leaf")
	if leaf == nil || len(leaf.Checks) != 1 || !leaf.Checks[0].Passed {
		t.Errorf("cookies end-to-end test = %+v", leaf)
	}
}
