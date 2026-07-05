package ui

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/history"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/logging"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/responsefmt"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/vars"
)

// withFolderVars prepends folder-scoped variables (#81) to a request's own
// variables so the resolver's request scope includes them; the request's own
// values come last and therefore win on a name clash (enabledRequestVars builds
// a map where later entries override). Returns own unchanged when there are no
// folder vars.
func withFolderVars(folder map[string]string, own []model.Variable) []model.Variable {
	if len(folder) == 0 {
		return own
	}
	out := make([]model.Variable, 0, len(folder)+len(own))
	for k, v := range folder {
		out = append(out, model.Variable{Enabled: true, Key: k, Value: v})
	}
	return append(out, own...)
}

// requestTemplateStrings collects every {{...}}-bearing string in a request so
// PromptVars can find the {{?Name}} markers a Send must ask the user about
// (#86): URL, body content, and the values of enabled headers / params / form
// fields, plus the common auth credential fields.
func requestTemplateStrings(r model.Request) []string {
	out := []string{r.URL, r.Body.Content}
	for _, h := range r.Headers {
		if h.Enabled {
			out = append(out, h.Value)
		}
	}
	for _, p := range r.Params {
		if p.Enabled {
			out = append(out, p.Value)
		}
	}
	for _, f := range r.Body.Form {
		if f.Enabled {
			out = append(out, f.Value)
		}
	}
	if a := r.Auth.Basic; a != nil {
		out = append(out, a.Username, a.Password)
	}
	if a := r.Auth.Bearer; a != nil {
		out = append(out, a.Token)
	}
	if a := r.Auth.APIKey; a != nil {
		out = append(out, a.Name, a.Value)
	}
	return out
}

// promptForVars asks the user for one value per {{?Name}} prompt key (#86),
// then re-enters send() with the values stashed in m.promptSnap. Cancelling
// aborts the Send.
func (m *MainUI) promptForVars(keys []string) {
	if m.win == nil {
		return
	}
	entries := make(map[string]*widget.Entry, len(keys))
	items := make([]*widget.FormItem, 0, len(keys))
	for _, k := range keys {
		e := widget.NewEntry()
		entries[k] = e
		items = append(items, widget.NewFormItem(vars.PromptLabel(k), e))
	}
	d := dialog.NewCustomConfirm("Enter request values", "Send", "Cancel",
		widget.NewForm(items...), func(ok bool) {
			if !ok {
				m.Status.SetText("Send cancelled")
				return
			}
			snap := make(map[string]string, len(keys))
			for k, e := range entries {
				snap[k] = e.Text
			}
			m.promptSnap = snap
			m.send()
		}, m.win)
	d.Resize(fyne.NewSize(420, 100+float32(60*len(keys))))
	d.Show()
}

// send executes the active edited request (or the bare method/URL if nothing is
// selected) off the UI goroutine, resolving {{vars}} against the active env.
// The pre-request script runs before httpclient builds the *http.Request so
// the script can mutate URL / method / body / headers / params; the
// post-response script runs once the response body is read so it can write
// extracted values into the session env overlay.
// sendOrAbort is the Send button's OnTapped handler. When a Send is in
// flight (sendCancel != nil), it cancels the in-flight context — the
// goroutine drains and the fyne.Do path resets the button via
// resetSendButton. Otherwise it starts a fresh Send. Both branches run
// on the UI thread; cancel() is idempotent so a double-tap is harmless.
func (m *MainUI) sendOrAbort() {
	if m.sendCancel != nil {
		m.sendCancel()
		return
	}
	m.send()
}

// resetSendButton restores the Send button to its default appearance
// and clears the abort state. UI-thread only — every Send teardown
// (success, error, panic, abort) routes through this helper inside a
// fyne.Do block.
func (m *MainUI) resetSendButton() {
	m.sendCancel = nil
	m.Send.SetIcon(themedIcon("location-arrow"))
	m.Send.SetText("")
	m.Send.Importance = widget.HighImportance
	m.Send.Refresh()
}

// setAbortButton switches the Send button into its in-flight "abort" look: a
// stop icon in warning importance. It stays icon-only (no "Abort" text) so the
// button keeps the same size as the default Send state — otherwise the wider
// text button reflows the address bar, which flickers when a quick Send toggles
// the button on and straight back off.
func (m *MainUI) setAbortButton() {
	m.Send.SetIcon(themedIcon("circle-stop"))
	m.Send.SetText("")
	m.Send.Importance = widget.WarningImportance
	m.Send.Refresh()
}

// snapshotRequest returns a copy of req with every reference-typed field
// detached from the original's backing arrays, so the off-UI send/chain worker
// (and the history snapshot #65) never shares mutable state with the live
// m.currentRequest the user keeps editing. It delegates to model.Request.Clone
// — the single home for that deep copy — so the field list can't drift. Note
// the send path reassigns req.Auth to the resolved EffectiveAuth afterwards and
// must re-Clone it there, since resolution re-aliases a live tree node's Auth.
func snapshotRequest(req model.Request) model.Request { return req.Clone() }

// sessionTransport returns the cached per-session *http.Transport, whose
// connection pool is reused across sends so repeated requests to one host skip
// TCP+TLS re-handshakes (#52). It is rebuilt only when InsecureSkipVerify (the
// one transport-affecting setting) changes; timeout/redirect policy live on the
// throwaway per-send Client. UI-thread only.
func (m *MainUI) sessionTransport() *http.Transport {
	insecure := m.sess.Settings().InsecureSkipVerify
	if m.httpTransport == nil || m.httpTransportInsecure != insecure {
		if m.httpTransport != nil {
			// Release the replaced pool's idle sockets now; otherwise they (and
			// their goroutine pairs) linger until the peer or timeout closes them.
			m.httpTransport.CloseIdleConnections()
		}
		m.httpTransport = httpclient.NewTransport(m.sess.Settings())
		m.httpTransportInsecure = insecure
	}
	return m.httpTransport
}

// guard runs fn and turns a panic into a surfaced error + log entry instead of
// a process exit. Fyne has no panic boundary around event handlers, so a panic
// in a dialog/action callback (e.g. a malformed-input parse) would otherwise
// hard-exit and lose unsaved work (#48). label identifies the action in the
// log and the error message.
func (m *MainUI) guard(label string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logging.L().Error("recovered panic in UI callback", "where", label, "recovered", r, "stack", string(debug.Stack()))
			err := fmt.Errorf("%s failed unexpectedly: %v", label, r)
			if m.win != nil {
				dialog.ShowError(err, m.win)
			} else if m.Status != nil {
				m.Status.SetText("Error: " + err.Error())
			}
		}
	}()
	fn()
}

// refreshEmptyState shows the first-run starter panel when no collection is
// loaded and hides it once one exists (#58). Safe to call before the panel is
// built (a no-op).
func (m *MainUI) refreshEmptyState() {
	if m.emptyState == nil {
		return
	}
	if len(m.sess.Collections()) == 0 {
		m.emptyState.Show()
	} else {
		m.emptyState.Hide()
	}
}

// showErrorBanner surfaces a request failure in the persistent, dismissible
// banner above the response tabs (#51). Unlike m.Status it does not auto-clear
// on the next status update.
func (m *MainUI) showErrorBanner(msg string) {
	if m.errorBanner == nil {
		return
	}
	m.errorBannerLabel.SetText(msg)
	m.errorBanner.Show()
	m.errorBanner.Refresh()
}

// hideErrorBanner clears the failure banner (on a successful response or an
// explicit dismiss).
func (m *MainUI) hideErrorBanner() {
	if m.errorBanner != nil {
		m.errorBanner.Hide()
		m.errorBanner.Refresh()
	}
}

// panicResponse builds the synthetic error response delivered when the Send
// worker recovers from a panic, so the originating tab reflects the failure
// instead of keeping its previous (now-stale) cached response (#110).
func panicResponse(r any) *tabResponse {
	msg := fmt.Sprintf("Send panic: %v", r)
	return &tabResponse{isError: true, errText: msg, status: msg}
}

func (m *MainUI) send() {
	if m.sendCancel != nil {
		// A Send is already in flight — Enter / URL OnSubmitted reach
		// this path directly (bypassing sendOrAbort), so guard here to
		// avoid leaking the in-flight cancel func when the field gets
		// overwritten. The button-tap dispatch goes through
		// sendOrAbort and would Abort instead.
		return
	}
	if m.streamCancel != nil {
		// An SSE stream is open (#74); it shares m.pv / m.Status, so don't start
		// a normal Send on top of it. The user stops the stream first.
		m.Status.SetText("Stop the stream first")
		return
	}
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}
	// A ws:// or wss:// URL is a WebSocket session, not an HTTP request (#72):
	// open the bidirectional message dialog instead of the request/response path.
	if isWebSocketURL(m.URL.Text) {
		m.openWebSocket()
		return
	}
	var req model.Request
	if m.currentRequest != nil {
		// The body editor's OnChanged is debounced; pull its live bytes now so a
		// Send fired before the settle still carries the latest body.
		m.syncBodyFromEditor()
		// Snapshot the live request on the UI goroutine, detaching its
		// slice-backed fields (Params/Headers/Body.Form/Chain) from
		// m.currentRequest's backing arrays — the user can keep editing those
		// tabs while the send/chain runs for ~ScriptTimeout on the worker, and
		// a shared backing array would be an unsynchronized read/write (race).
		req = snapshotRequest(*m.currentRequest)
		// Flatten any Inherit on the in-memory request copy via the session's
		// ancestor walk so httpclient sees the concrete auth. Clone it: Resolve
		// returns the request's (or an ancestor's) own Auth value, whose scheme
		// sub-struct pointers still alias the live tree node — the worker
		// dereferences them (auth.ResolveValues, history scrub) while the Auth
		// tab can mutate the same pointee on the UI thread, which would race.
		req.Auth = m.sess.EffectiveAuth(m.currentRequestID).Clone()
		// Fold the leaf's ancestor folder variables (#81) into its request scope
		// so {{vars}} resolve against them; the request's own values win.
		req.Variables = withFolderVars(m.sess.SnapshotAncestorVars(m.currentRequestID), req.Variables)
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected()), URL: m.URL.Text}
	}
	// #86: if the request references {{?Name}} prompt variables and we have not
	// yet collected them, pop a dialog and re-enter send() with the values
	// stashed in m.promptSnap. promptSnap is consumed (cleared) on this pass.
	promptSnap := m.promptSnap
	m.promptSnap = nil
	if promptSnap == nil {
		if keys := vars.PromptVars(requestTemplateStrings(req)...); len(keys) > 0 {
			m.promptForVars(keys)
			return
		}
	}
	// Snapshot env vars + auth state on the UI goroutine so the worker
	// goroutine can run for ~ScriptTimeout without racing against UI
	// mutations of s.cols / s.activeCol / Environment.Variables. The leaf's
	// slice fields are detached above; chain step targets come from the
	// SnapshotChainFinder copy below, so nothing the worker touches is shared.
	envSnap := m.sess.SnapshotActiveEnvVars()
	colSnap := m.sess.SnapshotActiveCollectionVars()
	dotEnvSnap := m.sess.SnapshotActiveDotEnvVars()
	globalSnap := m.sess.SnapshotGlobalVars()

	client := httpclient.NewWithTransport(m.sess.Settings(), m.sessionTransport())
	// Share the session cookie jar so Set-Cookie responses persist and matching
	// cookies are replayed on later sends — within a chain run and across runs
	// while the app is open (#91).
	client.SetCookieJar(m.sess.CookieJar())
	// If auth puts a key in a custom header, strip it on a host-changing
	// redirect so the credential doesn't leak to the new host (Go already
	// handles Authorization/Cookie).
	if k := req.Auth.APIKey; req.Auth.Type == model.AuthAPIKey && k != nil && k.Name != "" &&
		(k.Placement == model.APIKeyHeader || k.Placement == "") {
		client.SetCrossHostStripHeaders([]string{k.Name})
	}
	client.SetOAuth2Resolver(auth.NewOAuth2Resolver(
		m.sess.TokenCache(),
		nil, // default http.Client; settings-derived TLS/timeout intentionally not applied to the token endpoint
		m.sess.ActiveCollectionDir(),
		newAuthCodeStarter(),
	))
	rt := scripting.New(sessionEnvBridge{s: m.sess, base: envSnap})
	exec := chainExecutor{rt: rt, client: client, promptSnap: promptSnap, globalSnap: globalSnap, dotEnvSnap: dotEnvSnap, colSnap: colSnap, envSnap: envSnap, sess: m.sess}
	// Snapshot the active collection on the UI goroutine so the
	// chain runner reads from a frozen-at-Send-entry copy with
	// pre-flattened Auth — never races against UI-thread tree edits
	// and never sends a chain step with AuthInherit. Skipped for
	// chainless requests: the snapshot deep-copies the whole collection
	// (the dominant per-send allocation on large workspaces) and
	// chain.Resolve never consults the finder when the chain is empty.
	var finder chain.RequestFinder = nilFinder{}
	if len(req.Chain) > 0 {
		if snap := m.sess.SnapshotChainFinder(); snap != nil {
			finder = snap
		}
	}

	m.Status.SetText("Sending…")
	m.corsBanner.Hide()
	m.hideErrorBanner() // clear a prior failure when a new send starts
	// Diagnostic log with the URL redacted — never the resolved, secret-bearing
	// form (#49).
	logging.L().Info("send", "method", string(req.Method), "url", logging.RedactURL(req.URL), "name", req.Name)

	// Build the cancellable context on the UI thread so the click
	// handler (sendOrAbort) can call cancel() to abort an in-flight
	// Send. The button text swap signals the toggled mode to the user.
	ctx, cancel := context.WithCancel(context.Background())
	m.sendCancel = cancel
	// Abort mode: a warning-importance stop icon, icon-only so the button
	// stays the same size as the default Send state (see setAbortButton).
	m.setAbortButton()

	// Capture the pre-script's view of method+URL so we can flag any
	// mutation in the status line later.
	originalMethod, originalURL := req.Method, req.URL

	// Snapshot the overlay BEFORE the chain runs so we can roll back
	// any helena.env.set writes the chain landed if it then errored.
	// Failing chains shouldn't leak partial state into the next Send.
	preOverlay := m.sess.SnapshotEnvOverlay()

	// Capture the tab that initiated this Send so the async completion
	// attaches the response to it and only repaints the shared panel if it is
	// still active (the user may switch tabs mid-Send). nil when no tab is
	// open (a bare-URL Send).
	initTab := m.activeTab()

	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				m.sess.RestoreEnvOverlay(preOverlay)
				// Route the panic through the normal delivery path (#110) so the
				// originating tab's stale cached response is replaced with the
				// error and its in-flight state is cleared — not just a transient
				// status string that leaves the prior response in place.
				resp := panicResponse(r)
				fyne.Do(func() {
					m.resetSendButton()
					m.deliverResponse(initTab, resp)
				})
			}
		}()

		// Per-step progress feedback: chain.Resolve fires this once
		// before each ExecuteOnce on the worker goroutine; fyne.Do
		// marshals the status update to the UI thread.
		progress := func(step, total int, _, name string) {
			fyne.Do(func() {
				m.Status.SetText(fmt.Sprintf("Chain step %d/%d: %s", step, total, name))
			})
		}
		chainMap, chainConsole, chainErr := chain.Resolve(ctx, req, finder, exec, progress)
		if chainErr != nil {
			m.sess.RestoreEnvOverlay(preOverlay)
			status := "Chain error: " + chainErr.Error()
			if ctx.Err() == context.Canceled {
				status = "Aborted"
			}
			resp := &tabResponse{isError: true, errText: chainErr.Error(), status: status, console: chainConsole}
			fyne.Do(func() {
				m.resetSendButton()
				m.deliverResponse(initTab, resp)
			})
			return
		}

		// If the chain fired, swap the status back to a generic
		// "Sending…" so the user knows the leaf is now in flight and
		// not stuck on the last chain step.
		if len(chainMap) > 0 {
			fyne.Do(func() { m.Status.SetText("Sending: " + req.Name) })
		}

		view, leafConsole, leafErr := exec.ExecuteOnce(ctx, req, chainMap)
		consoleLines := append(chainConsole, leafConsole...)

		// Build the tab response off the UI goroutine (pure formatting, no
		// widget access); the fyne.Do block only resets the button + delivers.
		var resp *tabResponse
		if view.Response.StatusCode == 0 {
			// No HTTP completed → pre-script or HTTP failure.
			errText := ""
			if leafErr != nil {
				errText = leafErr.Error()
			}
			status := "Error: " + errText
			if ctx.Err() == context.Canceled {
				status = "Aborted"
			}
			resp = &tabResponse{isError: true, errText: errText, status: status, console: consoleLines}
		} else {
			// HTTP completed → render response. leafErr (if any) is a
			// post-script error; surfaced as a status-line suffix.
			status := fmt.Sprintf("%s · %s · %s",
				view.Response.Status,
				responsefmt.HumanSize(view.Response.Size),
				responsefmt.HumanDuration(view.Response.Duration))
			if view.Request.Method != string(originalMethod) || view.Request.URL != originalURL {
				status += fmt.Sprintf(" · sent %s %s", view.Request.Method, view.Request.URL)
			}
			if leafErr != nil {
				status += " · " + leafErr.Error()
			}
			cors := ""
			if view.Response.CORSWarning != "" {
				cors = "⚠ CORS: " + view.Response.CORSWarning
			}
			resp = &tabResponse{
				rawBody:     view.Response.Body,
				headersText: responsefmt.FormatHeaders(view.Response.Headers),
				status:      status,
				cors:        cors,
				console:     consoleLines,
			}
		}

		// Record the send in the history (#65). The store is thread-safe and
		// scrubs secrets itself, so this runs off the UI goroutine. WebSocket /
		// SSE sends use other paths and are not recorded.
		m.recordHistory(req, view, leafErr, ctx.Err() == context.Canceled)

		fyne.Do(func() {
			m.resetSendButton()
			m.deliverResponse(initTab, resp)
		})
	}()
}

// recordHistory appends a completed send to the session history (#65). The
// Method/URL shown prefer the resolved sent form for a meaningful list; the
// stored Request is the authored snapshot (with {{vars}}) so restore/resend
// re-resolves. The store scrubs secrets before persisting. A user-aborted send
// (canceled) is skipped — a Stop is not a real send, and logging it as an error
// would just clutter the list.
func (m *MainUI) recordHistory(req model.Request, view chain.View, leafErr error, canceled bool) {
	if canceled {
		return
	}
	e := history.Entry{Method: string(req.Method), URL: req.URL, Request: req}
	if view.Request.Method != "" {
		e.Method = view.Request.Method
	}
	if view.Request.URL != "" {
		e.URL = view.Request.URL
	}
	if view.Response.StatusCode != 0 {
		e.Status = view.Response.StatusCode
		e.Duration = view.Response.Duration
		e.Size = view.Response.Size
	} else if leafErr != nil {
		e.Err = leafErr.Error()
	}
	m.sess.History().Record(e)
}
