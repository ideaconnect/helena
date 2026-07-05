package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/sse"
	"github.com/idct/helena/internal/vars"
)

// MaxResponseBytes is the default cap on how much of a response body is
// buffered into memory when Settings.MaxResponseBytes is unset, so a huge or
// hostile response can't OOM the client (a desktop app, not a stream
// processor). Bodies past the cap are truncated and Response.Truncated is set.
// The effective cap is per-Client and configurable via the setting; see
// (*Client).maxResponseBytes.
const MaxResponseBytes = model.DefaultMaxResponseBytes

// readCapped reads up to max bytes from r, reporting whether the source had
// more (in which case the returned data is exactly max bytes). Reading one byte
// past the cap is how truncation is detected without buffering the whole body.
func readCapped(r io.Reader, max int64) (data []byte, truncated bool, err error) {
	data, err = io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > max {
		return data[:max], true, nil
	}
	return data, false, nil
}

// Response captures the outcome of executing a request.
//
// RequestURL and RequestBody record what actually went on the wire —
// the resolved URL (with {{vars}} substituted and query params merged)
// and the encoded body bytes (URL-encoded form for form-urlencoded,
// the wire multipart envelope for multipart, raw bytes otherwise).
// They are populated from the *http.Request that Do built, so they
// reflect any pre-script mutations the caller applied to the model
// before invoking Do. chain.View consumers (and the scripting
// `chain.<alias>.request.{url,body}` surface) read them so leaf
// scripts can inspect what each chain step actually sent.
type Response struct {
	StatusCode  int
	Status      string
	Proto       string
	Headers     http.Header
	Body        []byte
	Size        int64
	Duration    time.Duration
	CORSWarning string // advisory only; non-empty when a browser would likely block
	Truncated   bool   // body exceeded MaxResponseBytes and was cut off
	RequestURL  string
	RequestBody []byte
}

// Client executes model.Requests with behavior derived from settings.
type Client struct {
	settings       model.Settings
	http           *http.Client
	oauth2Resolver auth.OAuth2Resolver
	// crossHostStrip names request headers (e.g. a custom API key) to drop when
	// a redirect crosses to a different host, so credentials don't leak. Go
	// already strips Authorization/Cookie on cross-domain hops; this covers the
	// custom headers it forwards. Set per request by the caller.
	crossHostStrip []string
}

// SetCrossHostStripHeaders sets request headers to remove if a redirect leaves
// the original host (see Client.crossHostStrip).
func (c *Client) SetCrossHostStripHeaders(names []string) {
	c.crossHostStrip = names
}

// SetOAuth2Resolver installs the resolver consulted when a request's
// resolved auth is OAuth2. Nil disables OAuth2 (Apply will return
// ErrOAuth2NotImplemented). The resolver is shared across all requests
// executed through this Client so its own caching applies.
func (c *Client) SetOAuth2Resolver(r auth.OAuth2Resolver) {
	c.oauth2Resolver = r
}

// SetCookieJar installs the cookie jar the underlying *http.Client uses, so
// Do stores every Set-Cookie response and replays matching cookies on later
// requests — automatically across redirect hops. The caller holds the jar at
// session scope (like the cached transport, #52/#91), so cookies persist across
// the throwaway per-send Clients the UI builds. Nil disables the jar. Cookies
// the user set explicitly via a Cookie header are preserved: net/http appends
// jar cookies to it rather than replacing it.
func (c *Client) SetCookieJar(jar http.CookieJar) {
	c.http.Jar = jar
}

// CloseIdleConnections releases the client's pooled keep-alive sockets. Callers
// that own the transport (the headless runner, one-shot fetches) call it when
// their run ends so idle connections don't outlive the work that opened them;
// the UI's shared session transport deliberately keeps its pool (#52).
func (c *Client) CloseIdleConnections() {
	if t, ok := c.http.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}

// New builds a Client honoring the given settings: invalid-TLS tolerance,
// redirect policy, and request timeout.
// NewTransport builds the *http.Transport for the given settings (proxy from
// environment, optional insecure TLS). The transport owns the connection pool,
// so a caller that caches one across requests gets keep-alive reuse and skips
// repeated TCP+TLS handshakes. Only InsecureSkipVerify affects the transport;
// timeout/redirect policy live on the per-call *http.Client.
func NewTransport(s model.Settings) *http.Transport {
	// IdleConnTimeout is load-bearing: the zero value never expires idle
	// keep-alive sockets, so each one (plus its read/write goroutine pair)
	// would stay pinned for the whole session, one pair per host contacted.
	t := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		IdleConnTimeout: 90 * time.Second,
	}
	if s.InsecureSkipVerify {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit user opt-in
	}
	return t
}

// New constructs a Client with its own fresh transport (and connection pool).
func New(s model.Settings) *Client {
	return NewWithTransport(s, NewTransport(s))
}

// NewWithTransport constructs a Client that reuses the supplied transport, so
// its connection pool survives across the throwaway per-send Clients the UI
// builds (#52). The transport's TLS config is the caller's responsibility:
// pass one built for the same InsecureSkipVerify setting (see NewTransport). A
// nil transport falls back to a fresh one. Per-send state (crossHostStrip, the
// OAuth2 resolver) stays on the returned Client, never on the shared transport.
func NewWithTransport(s model.Settings, transport *http.Transport) *Client {
	if transport == nil {
		transport = NewTransport(s)
	}
	hc := &http.Client{Transport: transport}
	if s.TimeoutSeconds > 0 {
		hc.Timeout = time.Duration(s.TimeoutSeconds) * time.Second
	}
	c := &Client{settings: s, http: hc}
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !s.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 { // match net/http's default redirect cap
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		if len(via) == 0 {
			return nil
		}
		origin := via[0].URL
		// An https→http downgrade — even to the SAME host — must not carry
		// credentials over cleartext. net/http strips its own Authorization /
		// Cookie only on a host change (scheme is never compared), so a same-host
		// downgrade would otherwise forward them; treat the downgrade like a host
		// change and additionally drop Authorization that net/http would keep.
		downgrade := origin.Scheme == "https" && req.URL.Scheme == "http"
		// Once the chain leaves the originally-targeted host (or downgrades),
		// drop the caller-flagged credential headers so they aren't sent on.
		if req.URL.Host != origin.Host || downgrade {
			for _, h := range c.crossHostStrip {
				req.Header.Del(h)
			}
		}
		if downgrade {
			req.Header.Del("Authorization")
		}
		return nil
	}
	return c
}

// Build resolves variables and assembles an *http.Request. It returns
// the request, the encoded body bytes (nil for bodyless requests),
// and an error naming any unresolved {{variables}} so the caller can
// surface them. The body bytes are returned separately so callers
// that want to display the wire body (e.g. chain.View / scripting
// `chain.<alias>.request.body`) don't need to drain `req.GetBody`.
// Build is independent of any Client / settings — it's the pure
// "what would this request look like on the wire" path used by Do
// and by the exporter package.
//
// oauth2 may be nil; the OAuth2 case in auth.Apply then surfaces
// ErrOAuth2NotImplemented, which is the right thing for callers like the
// exporter that don't want to actually fetch a token at render time.
func Build(ctx context.Context, r model.Request, res *vars.Resolver, oauth2 auth.OAuth2Resolver) (*http.Request, []byte, error) {
	if res == nil {
		res = vars.New()
	}
	var missing []string
	resolve := func(s string) string {
		out, m := res.Resolve(s)
		missing = append(missing, m...)
		return out
	}

	rawURL := resolve(r.URL)
	body, contentType, err := buildBody(r, resolve)
	if err != nil {
		return nil, nil, err
	}

	type kv struct{ k, v string }
	var headers, params []kv
	for _, h := range model.EnabledPairs(r.Headers) {
		headers = append(headers, kv{resolve(h.Key), resolve(h.Value)})
	}
	for _, p := range model.EnabledPairs(r.Params) {
		params = append(params, kv{resolve(p.Key), resolve(p.Value)})
	}

	// Auth values share the same {{var}} substitution + missing-name reporting
	// as the rest of the request.
	resolvedAuth := auth.ResolveValues(r.Auth, resolve)

	if u := dedupe(missing); len(u) > 0 {
		return nil, nil, fmt.Errorf("unresolved variables: %s", strings.Join(u, ", "))
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if len(params) > 0 {
		// Append params preserving table order (and any query already on the
		// URL). url.Values.Encode() would sort keys alphabetically, silently
		// reordering the user's params on the wire.
		var b strings.Builder
		b.WriteString(u.RawQuery)
		for _, p := range params {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(p.k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(p.v))
		}
		u.RawQuery = b.String()
	}

	method := string(r.Method)
	if method == "" {
		method = http.MethodGet
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		b := body
		req.ContentLength = int64(len(b))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
	}

	hasContentType := false
	for _, h := range headers {
		if h.k == "" {
			continue
		}
		if strings.EqualFold(h.k, "Host") {
			req.Host = h.v
			continue
		}
		if strings.EqualFold(h.k, "Content-Type") {
			hasContentType = true
		}
		req.Header.Add(h.k, h.v)
	}
	if !hasContentType && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply auth last so it can see the headers the user explicitly set —
	// Basic/Bearer back off if Authorization is already present, and header
	// API-keys back off if their header name is taken. OAuth2 grants delegate
	// to the supplied resolver.
	if err := auth.Apply(ctx, req, resolvedAuth, oauth2); err != nil {
		return nil, nil, fmt.Errorf("auth: %w", err)
	}
	return req, body, nil
}

// sanitizeDoError redacts the resolved request URL embedded in a transport
// error before it reaches the UI status line. Go's *url.Error carries
// req.URL.String() verbatim, which can include userinfo (user:pass@host) or a
// {{token}}-interpolated query — credentials that must not leak into a
// user-facing error. The host stays visible (the useful diagnostic); only the
// secret-bearing parts are masked.
func sanitizeDoError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: redactURL(ue.URL), Err: ue.Err}
	}
	return err
}

// redactURL strips userinfo and the query string from a URL string, leaving
// scheme://host/path so the failed target is still identifiable without
// exposing embedded credentials. An unparseable input is fully redacted.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[redacted url]"
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	if u.RawQuery != "" {
		u.RawQuery = "redacted"
	}
	return u.String()
}

// maxResponseBytes is the effective body cap for this client: the configured
// Settings.MaxResponseBytes, or the built-in default when unset (<=0). Keeping
// the fallback here means an old config (or a zero-valued Settings) still gets
// a safe bound rather than an unbounded read.
func (c *Client) maxResponseBytes() int64 {
	if c.settings.MaxResponseBytes > 0 {
		return c.settings.MaxResponseBytes
	}
	return MaxResponseBytes
}

// Do builds and executes the request, fully reading the response body.
func (c *Client) Do(ctx context.Context, r model.Request, res *vars.Resolver) (*Response, error) {
	req, body, err := Build(ctx, r, res, c.oauth2Resolver)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, sanitizeDoError(err)
	}

	// HTTP Digest challenge/response (#75): the first request is sent without
	// credentials; on a 401 carrying a Digest challenge, compute the response
	// header and retry once with a freshly built request (so the body replays).
	if resp.StatusCode == http.StatusUnauthorized && r.Auth.Type == model.AuthDigest && r.Auth.Digest != nil {
		resolve := func(s string) string {
			if res == nil {
				return s
			}
			v, _ := res.Resolve(s)
			return v
		}
		creds := auth.ResolveValues(r.Auth, resolve).Digest
		hdr, ok, derr := auth.DigestRespond(resp.Header.Values("Www-Authenticate"), req.Method, req.URL.RequestURI(), *creds)
		if derr == nil && ok {
			if req2, body2, berr := Build(ctx, r, res, c.oauth2Resolver); berr == nil {
				req2.Header.Set("Authorization", hdr)
				if resp2, err2 := c.http.Do(req2); err2 == nil {
					// Close the original 401 only now that the retry supersedes
					// it — if the retry had failed we must leave its body open so
					// the read below surfaces the original 401, not a
					// "read on closed body" error.
					_ = resp.Body.Close()
					resp, body = resp2, body2
				}
			}
		}
	}

	// NTLM (#78) is a multi-round connection handshake: on a 401 inviting NTLM,
	// send the type-1 NEGOTIATE, read the type-2 CHALLENGE from the next 401, and
	// re-send with the type-3 AUTHENTICATE. The three exchanges must reuse one
	// keep-alive connection, so each intermediate body is drained to EOF + closed
	// to return the connection to the transport's pool.
	if resp.StatusCode == http.StatusUnauthorized && r.Auth.Type == model.AuthNTLM && r.Auth.NTLM != nil &&
		auth.NTLMOffered(resp.Header.Values("Www-Authenticate")) {
		resolve := func(s string) string {
			if res == nil {
				return s
			}
			v, _ := res.Resolve(s)
			return v
		}
		creds := auth.ResolveValues(r.Auth, resolve).NTLM
		if resp2, body2, ok := c.ntlmHandshake(ctx, r, res, resp, *creds); ok {
			resp, body = resp2, body2
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Bound the read so an oversized/hostile body can't OOM the app.
	data, truncated, err := readCapped(resp.Body, c.maxResponseBytes())
	dur := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	out := &Response{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		Proto:       resp.Proto,
		Headers:     resp.Header,
		Body:        data,
		Size:        int64(len(data)),
		Truncated:   truncated,
		Duration:    dur,
		RequestURL:  req.URL.String(),
		RequestBody: body,
	}
	if c.settings.CORSWarning {
		out.CORSWarning = corsAdvisory(req.Header.Get("Origin"), req.Header.Get("Cookie") != "", resp.Header)
	}
	return out, nil
}

// ntlmHandshake runs the NTLMv2 NEGOTIATE → CHALLENGE → AUTHENTICATE exchange
// (#78) after an initial 401 invited NTLM. Returns the final response + replayed
// body, or ok=false (leaving the caller to surface the original 401) if any step
// fails. Each request is freshly built via Build so its body replays; the
// transport reuses the same keep-alive connection across the steps.
func (c *Client) ntlmHandshake(ctx context.Context, r model.Request, res *vars.Resolver, initial *http.Response, creds model.NTLMAuth) (*http.Response, []byte, bool) {
	// Capture the initial 401's body before draining its connection back to the
	// pool. On any failure below we restore it so the caller can surface the
	// original 401 — otherwise its body is closed and the caller's read errors
	// with "read on closed body" (e.g. a proxy that drops connection affinity
	// gives an unparseable type-2 challenge).
	initialBody := c.drainBodyBytes(initial)
	fail := func() (*http.Response, []byte, bool) {
		initial.Body = io.NopCloser(bytes.NewReader(initialBody))
		return nil, nil, false
	}

	req1, _, err := Build(ctx, r, res, c.oauth2Resolver)
	if err != nil {
		return fail()
	}
	req1.Header.Set("Authorization", auth.NTLMNegotiateHeader())
	resp1, err := c.http.Do(req1)
	if err != nil {
		return fail()
	}
	challenge, ok := auth.NTLMChallenge(resp1.Header.Values("Www-Authenticate"))
	c.drainBody(resp1)
	if !ok {
		return fail()
	}

	hdr, err := auth.NTLMAuthenticateHeader(challenge, creds)
	if err != nil {
		return fail()
	}
	req2, body2, err := Build(ctx, r, res, c.oauth2Resolver)
	if err != nil {
		return fail()
	}
	req2.Header.Set("Authorization", hdr)
	resp2, err := c.http.Do(req2)
	if err != nil {
		return fail()
	}
	return resp2, body2, true
}

// drainBody reads an intermediate response to EOF (bounded) and closes it so the
// transport can reuse the underlying connection — required for the NTLM
// handshake's connection affinity. A body larger than the cap is left unread
// (the connection won't be reused, and the handshake degrades to the 401).
func (c *Client) drainBody(resp *http.Response) { _ = c.drainBodyBytes(resp) }

// drainBodyBytes is drainBody that also returns the (bounded) bytes it read, so
// a caller can reconstruct the response body after the connection is freed.
func (c *Client) drainBodyBytes(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes()))
	_ = resp.Body.Close()
	return data
}

// StreamMeta carries the response metadata available once an SSE stream opens,
// before any events arrive (#74).
type StreamMeta struct {
	StatusCode int
	Status     string
	Headers    http.Header
	RequestURL string
}

// Stream sends r and delivers Server-Sent Events to onEvent until the stream
// ends, onEvent returns false, or ctx is cancelled (#74). The request advertises
// Accept: text/event-stream (unless the user set Accept). onOpen, if non-nil, is
// called once with the response metadata before the first event. The response
// body is read incrementally — not capped like Do — so a long-lived stream
// works; cancel ctx to stop it. A non-2xx status returns an error without
// streaming. TimeoutSeconds bounds only the connect + response-header phase:
// unlike Do, an open stream is never killed by the client-wide timeout (that
// deadline covers the body read, which for SSE is the stream itself).
func (c *Client) Stream(ctx context.Context, r model.Request, res *vars.Resolver, onOpen func(StreamMeta), onEvent func(sse.Event) bool) error {
	req, _, err := Build(ctx, r, res, c.oauth2Resolver)
	if err != nil {
		return err
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}
	// Streams are exempt from the client-wide Timeout: it covers the entire
	// exchange INCLUDING the body read, and for SSE the body read is the
	// stream itself — the default 30 s would kill every stream mid-flight. A
	// shallow-copied client shares the transport/jar/redirect policy but
	// drops the deadline; the connect + response-header phase stays bounded
	// by the same duration via a timer on the request context (stopped the
	// moment headers arrive), and from then on the caller's ctx — the Stop
	// button — is the stream's only lifetime control.
	hc := *c.http
	hc.Timeout = 0
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req = req.WithContext(sctx)
	// timedOut distinguishes the header timer's cancel from the caller's: the
	// timer can fire in the gap between Do returning success and Stop (Stop
	// cannot un-fire it), which would otherwise kill the just-opened stream
	// with a generic "context canceled" — off the documented contract.
	var timedOut atomic.Bool
	var headerTimer *time.Timer
	if d := c.http.Timeout; d > 0 {
		headerTimer = time.AfterFunc(d, func() { timedOut.Store(true); cancel() })
	}
	headerTimeoutErr := func() error {
		return fmt.Errorf("sse: no response within %s", c.http.Timeout)
	}
	resp, err := hc.Do(req)
	if headerTimer != nil {
		headerTimer.Stop()
	}
	// One branch for both timeout shapes: Do failing because the timer
	// cancelled sctx, AND the race where the timer fired in the Do-return ↔
	// Stop gap (Do succeeded but the body is already doomed) — either way,
	// report the header timeout deterministically.
	if ctx.Err() == nil && timedOut.Load() {
		if err == nil {
			_ = resp.Body.Close()
		}
		return headerTimeoutErr()
	}
	if err != nil {
		return sanitizeDoError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sse: server returned %s", resp.Status)
	}
	if onOpen != nil {
		onOpen(StreamMeta{StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Header, RequestURL: req.URL.String()})
	}

	p := sse.NewParser(resp.Body)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ev, err := p.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// A read aborted by ctx cancellation surfaces as the ctx error.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if timedOut.Load() {
				// The header timer's late cancel aborted the read (see above)
				// — name the real cause, not a generic "context canceled".
				return headerTimeoutErr()
			}
			return fmt.Errorf("sse: %w", err)
		}
		if onEvent != nil && !onEvent(ev) {
			return nil
		}
	}
}

// buildBody serializes r.Body into bytes plus the matching Content-Type. The
// returned content type is only used when the request has no explicit
// Content-Type header.
func buildBody(r model.Request, resolve func(string) string) (body []byte, contentType string, err error) {
	switch r.Body.Type {
	case "", model.BodyNone:
		return nil, "", nil
	case model.BodyJSON, model.BodyXML, model.BodyText:
		return []byte(resolve(r.Body.Content)), r.Body.Type.ContentType(), nil
	case model.BodyForm:
		if len(r.Body.Form) > 0 {
			form := url.Values{}
			for _, f := range model.EnabledPairs(r.Body.Form) {
				form.Add(resolve(f.Key), resolve(f.Value))
			}
			return []byte(form.Encode()), model.BodyForm.ContentType(), nil
		}
		// No structured form fields set — fall back to the raw Content so the
		// body-content editor works for form-urlencoded too.
		return []byte(resolve(r.Body.Content)), model.BodyForm.ContentType(), nil
	case model.BodyMultipart:
		// Encode the structured Body.Form pairs as multipart/form-data. The
		// Content-Type carries the generated boundary, so it must be the writer's
		// FormDataContentType (the model's static ContentType can't know it).
		// File-part uploads are a separate follow-up (the model has no file kind
		// yet); text fields are encoded here.
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for _, f := range model.EnabledPairs(r.Body.Form) {
			if err := w.WriteField(resolve(f.Key), resolve(f.Value)); err != nil {
				return nil, "", fmt.Errorf("multipart: %w", err)
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", fmt.Errorf("multipart: %w", err)
		}
		return buf.Bytes(), w.FormDataContentType(), nil
	case model.BodyGraphQL:
		// Send a GraphQL-over-HTTP JSON envelope (#70): {"query":…,"variables":…}.
		// Body.Content is the query; Body.GraphQLVariables is the raw JSON object
		// of variables (omitted when blank). {{vars}} resolve in both. Invalid
		// variables JSON is surfaced rather than sent as a broken body.
		query, err := json.Marshal(resolve(r.Body.Content))
		if err != nil {
			return nil, "", fmt.Errorf("graphql query: %w", err)
		}
		envelope := `{"query":` + string(query)
		if vars := strings.TrimSpace(resolve(r.Body.GraphQLVariables)); vars != "" {
			if !json.Valid([]byte(vars)) {
				return nil, "", fmt.Errorf("graphql variables are not valid JSON")
			}
			envelope += `,"variables":` + vars
		}
		envelope += `}`
		return []byte(envelope), model.BodyGraphQL.ContentType(), nil
	case model.BodyFile:
		// Send the exact bytes of the chosen file (#24). No file path → no body
		// (the user hasn't picked one yet). The advertised Content-Type comes
		// from the request, defaulting to application/octet-stream; a user-set
		// Content-Type header still wins (see the hasContentType check above).
		if r.Body.FilePath == "" {
			return nil, "", nil
		}
		data, err := os.ReadFile(r.Body.FilePath)
		if err != nil {
			return nil, "", fmt.Errorf("read body file %q: %w", r.Body.FilePath, err)
		}
		ct := r.Body.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		return data, ct, nil
	default:
		return []byte(resolve(r.Body.Content)), "", nil
	}
}

// corsAdvisory returns a non-empty message when a browser would block a request
// from origin given the response's CORS headers. A native client ignores CORS;
// this is purely advisory.
// credentialed is true when the request carries cookies — CORS "credentials
// mode", where a wildcard Access-Control-Allow-Origin is rejected and an
// echoed origin additionally needs Access-Control-Allow-Credentials: true.
func corsAdvisory(origin string, credentialed bool, h http.Header) string {
	if origin == "" {
		return ""
	}
	allow := h.Get("Access-Control-Allow-Origin")
	switch {
	case allow == "":
		return fmt.Sprintf("a browser would block this: Origin %q sent, but the response has no Access-Control-Allow-Origin", origin)
	case allow == "*":
		if credentialed {
			return fmt.Sprintf("a browser would block this: a credentialed request from Origin %q can't use the wildcard Access-Control-Allow-Origin: *", origin)
		}
		return ""
	case strings.EqualFold(allow, origin):
		if credentialed && !strings.EqualFold(h.Get("Access-Control-Allow-Credentials"), "true") {
			return fmt.Sprintf("a browser would block this: a credentialed request from Origin %q needs Access-Control-Allow-Credentials: true", origin)
		}
		return ""
	default:
		return fmt.Sprintf("a browser would block this: Origin %q is not allowed by Access-Control-Allow-Origin %q", origin, allow)
	}
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
