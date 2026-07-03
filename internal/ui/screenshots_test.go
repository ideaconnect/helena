package ui

// Screenshot generator (dev tool, not a unit test). Renders the *real* Helena
// UI against a small fake HTTP API using Fyne's software canvas and writes PNG
// captures for the project website. It is skipped unless HELENA_SHOTS points at
// an output directory, so it never runs in the normal suite or under CI.
//
//	make screenshots            # writes website/assets/img/*.png
//	HELENA_SHOTS=/tmp/shots go test ./internal/ui -run TestGenerateScreenshots
//
// The captures are genuine: real widgets, the real Helena theme + fonts, and
// real response bytes fetched from the fake API through the same chainExecutor
// path the live Send button uses. Only the goroutine/fyne.Do marshalling is
// skipped (it has no effect on pixels) so the generator stays deterministic and
// race-free on a single goroutine.

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/responsefmt"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/storage"
)

// shotScale renders captures at 2× so they stay crisp on HiDPI displays.
const shotScale = 2.0

// shotSize is the logical window size; mirrors the app's default (main.go).
var shotSize = fyne.NewSize(1180, 760)

// fakeAPI is a tiny in-memory JSON API the screenshots drive, so the captures
// show real request/response round-trips rather than mockups.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		b, _ := json.MarshalIndent(v, "", "  ")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Server", "helena-demo")
		w.WriteHeader(code)
		_, _ = w.Write(b)
	}
	user := map[string]any{
		"id":       42,
		"name":     "Ada Lovelace",
		"email":    "ada@example.com",
		"role":     "admin",
		"active":   true,
		"tags":     []string{"founder", "mathematician"},
		"created":  "1843-10-01T09:24:00Z",
		"location": map[string]any{"city": "London", "country": "GB"},
	}
	mux.HandleFunc("/users/42", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, user)
	})
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusCreated, map[string]any{
				"id": 1024, "name": "Grace Hopper", "role": "engineer",
				"created": "2026-06-28T10:21:00Z",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"page": 1, "total": 2,
			"users": []any{user, map[string]any{"id": 7, "name": "Alan Turing", "role": "engineer"}},
		})
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"token": "eyJhbGciOiJIUzI1NiJ9.demo.signature", "expiresIn": 3600,
		})
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": "ord_1099", "status": "confirmed", "total": 4200,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// sampleCollection writes a realistic demo collection rooted at base (the fake
// API URL) and returns its directory. The tree gives the sidebar real content.
func sampleCollection(t *testing.T, base string) string {
	t.Helper()
	hdr := []model.KeyValue{
		{Key: "Accept", Value: "application/json", Enabled: true},
		{Key: "X-Request-Id", Value: "{{$guid}}", Enabled: true},
	}
	c := model.Collection{
		Name: "Acme API",
		Requests: []model.Request{
			{ID: "health", Name: "Health check", Method: model.GET, URL: base + "/healthz"},
		},
		Folders: []model.Folder{
			{Name: "Users", Requests: []model.Request{
				{ID: "get-user", Name: "Get user", Method: model.GET, URL: base + "/users/42", Headers: hdr},
				{ID: "list-users", Name: "List users", Method: model.GET, URL: base + "/users"},
				{ID: "create-user", Name: "Create user", Method: model.POST, URL: base + "/users",
					Headers: hdr,
					Body: model.Body{Type: model.BodyJSON,
						Content: "{\n  \"name\": \"Grace Hopper\",\n  \"role\": \"engineer\"\n}"}},
			}},
			{Name: "Auth", Requests: []model.Request{
				{ID: "login", Name: "Login", Method: model.POST, URL: base + "/login",
					Body: model.Body{Type: model.BodyJSON,
						Content: "{\n  \"email\": \"ada@example.com\",\n  \"password\": \"{{PASSWORD}}\"\n}"}},
				{ID: "profile", Name: "My profile", Method: model.GET, URL: base + "/users/42",
					Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "{{TOKEN}}"}}},
			}},
			{Name: "Orders", Requests: []model.Request{
				// Runs "Auth/Login" first (aliased "auth") and reuses its token.
				{ID: "place-order", Name: "Place order", Method: model.POST, URL: base + "/orders",
					Chain:   []model.ChainStep{{Alias: "auth", Request: "Auth/Login", RequestID: "login"}},
					Headers: []model.KeyValue{{Key: "Authorization", Value: "Bearer {{chain.auth.response.json.token}}", Enabled: true}},
					Body: model.Body{Type: model.BodyJSON,
						Content: "{\n  \"sku\": \"HLN-42\",\n  \"qty\": 2\n}"}},
			}},
		},
	}
	dir := filepath.Join(t.TempDir(), "acme-api")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save sample collection: %v", err)
	}
	return dir
}

// shotUI builds a fresh, themed MainUI bound to the sample collection and a
// resized capture window. Returns the UI, the window, and the session.
func shotUI(t *testing.T, dir string) (*MainUI, fyne.Window, *session.Session) {
	t.Helper()
	a := test.NewApp()
	ApplyTheme(a, model.ThemeDark)

	sess, err := session.New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	sess.SetActiveCollection(0)

	m := NewMainUI(sess)
	m.Tree.OpenAllBranches() // show the collection's folders + requests in the sidebar
	w := test.NewWindow(m.Root())
	m.SetWindow(w)
	w.Resize(shotSize)
	if sc, ok := w.Canvas().(interface{ SetScale(float32) }); ok {
		sc.SetScale(shotScale)
	}
	return m, w, sess
}

// openByID loads the request with the given ID into the editor, mirroring a
// sidebar click.
func openByID(t *testing.T, m *MainUI, sess *session.Session, dir, id string) {
	t.Helper()
	nodeID, _, ok := sess.LocateRequest(dir, id)
	if !ok {
		t.Fatalf("request %q not found in %q", id, dir)
	}
	m.openOrActivate(nodeID)
	m.Tree.Select(nodeID) // highlight the active request in the sidebar
}

// sendSync runs the current request synchronously through the real execution
// path and pushes the result into the response panel exactly as a live Send
// would, so the capture shows genuine response bytes.
func sendSync(t *testing.T, m *MainUI, sess *session.Session) {
	t.Helper()
	if m.currentRequest == nil {
		t.Fatal("sendSync: no current request")
	}
	req := snapshotRequest(*m.currentRequest)
	req.Auth = sess.EffectiveAuth(m.currentRequestID)

	client := httpclient.New(sess.Settings())
	rt := scripting.New(sessionEnvBridge{s: sess})
	exec := chainExecutor{rt: rt, client: client, sess: sess}
	view, console, err := exec.ExecuteOnce(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("send %q: %v", m.currentRequestID, err)
	}
	status := fmt.Sprintf("%s · %s · %s",
		view.Response.Status,
		responsefmt.HumanSize(view.Response.Size),
		responsefmt.HumanDuration(view.Response.Duration))
	m.applyResponse(&tabResponse{
		rawBody:     view.Response.Body,
		headersText: responsefmt.FormatHeaders(view.Response.Headers),
		status:      status,
		console:     console,
	})
}

// sendChainSync resolves the current request's chain (running each prior
// request) and then the leaf, exactly like a live Send, and pushes the result
// into the response panel — so a chaining capture shows a real round-trip where
// the leaf reused a value produced by an earlier request.
func sendChainSync(t *testing.T, m *MainUI, sess *session.Session) {
	t.Helper()
	req := snapshotRequest(*m.currentRequest)
	req.Auth = sess.EffectiveAuth(m.currentRequestID)
	client := httpclient.New(sess.Settings())
	rt := scripting.New(sessionEnvBridge{s: sess})
	exec := chainExecutor{rt: rt, client: client, sess: sess}
	var finder chain.RequestFinder = nilFinder{}
	if snap := sess.SnapshotChainFinder(); snap != nil {
		finder = snap
	}
	noop := func(int, int, string, string) {}
	chainMap, chainConsole, err := chain.Resolve(context.Background(), req, finder, exec, noop)
	if err != nil {
		t.Fatalf("chain resolve: %v", err)
	}
	view, leafConsole, err := exec.ExecuteOnce(context.Background(), req, chainMap)
	if err != nil {
		t.Fatalf("chain leaf: %v", err)
	}
	status := fmt.Sprintf("%s · %s · %s",
		view.Response.Status, responsefmt.HumanSize(view.Response.Size),
		responsefmt.HumanDuration(view.Response.Duration))
	m.applyResponse(&tabResponse{
		rawBody:     view.Response.Body,
		headersText: responsefmt.FormatHeaders(view.Response.Headers),
		status:      status,
		console:     append(chainConsole, leafConsole...),
	})
}

// capture renders the window to a PNG under outDir. A short settle Refresh pass
// flushes any pending widget layout before the snapshot.
func capture(t *testing.T, w fyne.Window, outDir, name string) {
	t.Helper()
	w.Content().Refresh()
	img := w.Canvas().Capture()
	path := filepath.Join(outDir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	t.Logf("wrote %s (%dx%d)", path, img.Bounds().Dx(), img.Bounds().Dy())
}

func TestGenerateScreenshots(t *testing.T) {
	outDir := os.Getenv("HELENA_SHOTS")
	if outDir == "" {
		t.Skip("set HELENA_SHOTS=<output dir> to (re)generate website screenshots")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	srv := fakeAPI(t)
	dir := sampleCollection(t, srv.URL)

	// Hero: compose a POST with a JSON body and show the 201 response.
	{
		m, w, sess := shotUI(t, dir)
		openByID(t, m, sess, dir, "create-user")
		m.Request.SelectIndex(0) // Body
		sendSync(t, m, sess)
		m.Response.SelectIndex(0) // Body
		capture(t, w, outDir, "app-hero.png")
	}

	// GET + pretty JSON object on the response Body tab.
	{
		m, w, sess := shotUI(t, dir)
		openByID(t, m, sess, dir, "get-user")
		m.Request.SelectIndex(2) // Headers (shows the request's headers)
		sendSync(t, m, sess)
		m.Response.SelectIndex(0) // Body
		capture(t, w, outDir, "shot-request.png")
	}

	// Auth: the Auth tab with a Bearer token referencing an externalised secret.
	{
		m, w, sess := shotUI(t, dir)
		// Resolve {{TOKEN}} on the wire while the editor still shows the template,
		// demonstrating the externalised-secret workflow (#42).
		sess.SetEnvOverlay("TOKEN", "demo-bearer-token")
		openByID(t, m, sess, dir, "profile")
		m.Request.SelectIndex(1) // Auth
		sendSync(t, m, sess)
		m.Response.SelectIndex(0)
		capture(t, w, outDir, "shot-auth.png")
	}

	// Chaining: the Chain tab shows a prior request bound as `auth`, and the
	// response is the order created after reusing the chained login token.
	{
		m, w, sess := shotUI(t, dir)
		sess.SetEnvOverlay("PASSWORD", "demo-password")
		openByID(t, m, sess, dir, "place-order")
		m.Request.SelectIndex(6) // Chain
		sendChainSync(t, m, sess)
		m.Response.SelectIndex(0) // Body
		capture(t, w, outDir, "shot-chain.png")
	}
}
