// Package exporter renders a request as runnable snippets for other tools
// (cURL and WGET today; more languages later). It reuses httpclient.Build so
// the exported command matches what Helena would actually send: same variable
// resolution, same query/header/body construction, same auto Content-Type.
package exporter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// ToCurl renders r as a multi-line cURL command. Variables in URL / headers /
// params / body are resolved via res; insecure-TLS, follow-redirects, and
// timeout flags are taken from s.
func ToCurl(r model.Request, res *vars.Resolver, s model.Settings) (string, error) {
	// nil OAuth2 resolver — render the request as it stands without firing
	// a live token fetch at export time.
	req, _, err := httpclient.Build(context.Background(), r, res, nil)
	if err != nil {
		return "", err
	}
	return renderCurl(req, s)
}

// ToWget renders r as a wget command with the same semantics as ToCurl.
func ToWget(r model.Request, res *vars.Resolver, s model.Settings) (string, error) {
	req, _, err := httpclient.Build(context.Background(), r, res, nil)
	if err != nil {
		return "", err
	}
	return renderWget(req, s)
}

func renderCurl(req *http.Request, s model.Settings) (string, error) {
	lines := []string{fmt.Sprintf("curl -X %s %s", req.Method, alwaysQuote(req.URL.String()))}

	if s.InsecureSkipVerify {
		lines = append(lines, "-k")
	}
	if s.FollowRedirects {
		lines = append(lines, "-L")
	}
	if s.TimeoutSeconds > 0 {
		lines = append(lines, fmt.Sprintf("--max-time %d", s.TimeoutSeconds))
	}
	for _, k := range sortedHeaderKeys(req.Header) {
		for _, v := range req.Header[k] {
			lines = append(lines, fmt.Sprintf("-H %s", shellQuote(k+": "+v)))
		}
	}
	// Go keeps a custom Host header out of req.Header (it lives on req.Host), so
	// emit it explicitly or the snippet would hit the wrong virtual host.
	if req.Host != "" && req.Host != req.URL.Host {
		lines = append(lines, fmt.Sprintf("-H %s", shellQuote("Host: "+req.Host)))
	}

	body, err := readBodyBytes(req)
	if err != nil {
		return "", err
	}
	if len(body) > 0 {
		lines = append(lines, fmt.Sprintf("--data-raw %s", shellQuote(string(body))))
	}
	return strings.Join(lines, " \\\n  "), nil
}

func renderWget(req *http.Request, s model.Settings) (string, error) {
	lines := []string{fmt.Sprintf("wget --method=%s", req.Method)}

	if s.InsecureSkipVerify {
		lines = append(lines, "--no-check-certificate")
	}
	if !s.FollowRedirects {
		lines = append(lines, "--max-redirect=0")
	}
	if s.TimeoutSeconds > 0 {
		lines = append(lines, fmt.Sprintf("--timeout=%d", s.TimeoutSeconds))
	}
	for _, k := range sortedHeaderKeys(req.Header) {
		for _, v := range req.Header[k] {
			lines = append(lines, fmt.Sprintf("--header=%s", shellQuote(k+": "+v)))
		}
	}
	// Custom Host header lives on req.Host, not req.Header — emit it explicitly.
	if req.Host != "" && req.Host != req.URL.Host {
		lines = append(lines, fmt.Sprintf("--header=%s", shellQuote("Host: "+req.Host)))
	}

	body, err := readBodyBytes(req)
	if err != nil {
		return "", err
	}
	if len(body) > 0 {
		lines = append(lines, fmt.Sprintf("--body-data=%s", shellQuote(string(body))))
	}
	lines = append(lines, "-qO-") // write to stdout
	lines = append(lines, alwaysQuote(req.URL.String()))
	return strings.Join(lines, " \\\n  "), nil
}

func sortedHeaderKeys(h http.Header) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func readBodyBytes(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.GetBody == nil {
		return nil, nil
	}
	rc, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// shellQuote returns s safely quoted for a POSIX shell. Strings containing only
// shell-safe characters are returned bare; everything else is wrapped in single
// quotes, with embedded single quotes escaped as '\”.
func shellQuote(s string) string {
	if !needsShellQuote(s) {
		return s
	}
	return alwaysQuote(s)
}

// alwaysQuote unconditionally single-quotes s. Used for URLs so exported
// commands look consistent regardless of whether the URL happens to contain
// special characters.
func alwaysQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsShellQuote(s string) bool {
	if s == "" {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == '/', c == ':', c == '+', c == '%', c == ',':
		default:
			return true
		}
	}
	return false
}
