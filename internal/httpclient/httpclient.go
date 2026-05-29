package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// Response captures the outcome of executing a request.
type Response struct {
	StatusCode  int
	Status      string
	Proto       string
	Headers     http.Header
	Body        []byte
	Size        int64
	Duration    time.Duration
	CORSWarning string // advisory only; non-empty when a browser would likely block
}

// Client executes model.Requests with behavior derived from settings.
type Client struct {
	settings       model.Settings
	http           *http.Client
	oauth2Resolver auth.OAuth2Resolver
}

// SetOAuth2Resolver installs the resolver consulted when a request's
// resolved auth is OAuth2. Nil disables OAuth2 (Apply will return
// ErrOAuth2NotImplemented). The resolver is shared across all requests
// executed through this Client so its own caching applies.
func (c *Client) SetOAuth2Resolver(r auth.OAuth2Resolver) {
	c.oauth2Resolver = r
}

// New builds a Client honoring the given settings: invalid-TLS tolerance,
// redirect policy, and request timeout.
func New(s model.Settings) *Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if s.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit user opt-in
	}
	hc := &http.Client{Transport: transport}
	if s.TimeoutSeconds > 0 {
		hc.Timeout = time.Duration(s.TimeoutSeconds) * time.Second
	}
	if !s.FollowRedirects {
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &Client{settings: s, http: hc}
}

// Build resolves variables and assembles an *http.Request. It returns an error
// naming any unresolved {{variables}} so the caller can surface them. Build is
// independent of any Client / settings — it's the pure "what would this
// request look like on the wire" path used by Do and by the exporter package.
//
// oauth2 may be nil; the OAuth2 case in auth.Apply then surfaces
// ErrOAuth2NotImplemented, which is the right thing for callers like the
// exporter that don't want to actually fetch a token at render time.
func Build(ctx context.Context, r model.Request, res *vars.Resolver, oauth2 auth.OAuth2Resolver) (*http.Request, error) {
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
		return nil, err
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
		return nil, fmt.Errorf("unresolved variables: %s", strings.Join(u, ", "))
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if len(params) > 0 {
		q := u.Query()
		for _, p := range params {
			q.Add(p.k, p.v)
		}
		u.RawQuery = q.Encode()
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
		return nil, err
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
		return nil, fmt.Errorf("auth: %w", err)
	}
	return req, nil
}

// Do builds and executes the request, fully reading the response body.
func (c *Client) Do(ctx context.Context, r model.Request, res *vars.Resolver) (*Response, error) {
	req, err := Build(ctx, r, res, c.oauth2Resolver)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	dur := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	out := &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Proto:      resp.Proto,
		Headers:    resp.Header,
		Body:       data,
		Size:       int64(len(data)),
		Duration:   dur,
	}
	if c.settings.CORSWarning {
		out.CORSWarning = corsAdvisory(req.Header.Get("Origin"), resp.Header)
	}
	return out, nil
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
		return nil, "", fmt.Errorf("multipart bodies are not supported yet")
	default:
		return []byte(resolve(r.Body.Content)), "", nil
	}
}

// corsAdvisory returns a non-empty message when a browser would block a request
// from origin given the response's CORS headers. A native client ignores CORS;
// this is purely advisory.
func corsAdvisory(origin string, h http.Header) string {
	if origin == "" {
		return ""
	}
	allow := h.Get("Access-Control-Allow-Origin")
	switch {
	case allow == "":
		return fmt.Sprintf("a browser would block this: Origin %q sent, but the response has no Access-Control-Allow-Origin", origin)
	case allow == "*" || strings.EqualFold(allow, origin):
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
