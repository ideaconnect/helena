package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// ToFetch renders r as a browser/Node JavaScript fetch() call. Variables are
// resolved via res exactly as for ToCurl; TLS-verify and timeout settings have
// no portable fetch equivalent and are omitted.
func ToFetch(r model.Request, res *vars.Resolver, s model.Settings) (string, error) {
	req, _, err := httpclient.Build(context.Background(), r, res, nil)
	if err != nil {
		return "", err
	}
	return renderFetch(req)
}

// ToPython renders r as a Python `requests` snippet, mapping insecure-TLS,
// follow-redirects, and timeout settings to the matching keyword arguments.
func ToPython(r model.Request, res *vars.Resolver, s model.Settings) (string, error) {
	req, _, err := httpclient.Build(context.Background(), r, res, nil)
	if err != nil {
		return "", err
	}
	return renderPython(req, s)
}

// ToGo renders r as a Go net/http program. A custom http.Client is emitted only
// when a setting requires it (insecure TLS, a timeout, or redirect suppression);
// otherwise the snippet uses http.DefaultClient.
func ToGo(r model.Request, res *vars.Resolver, s model.Settings) (string, error) {
	req, _, err := httpclient.Build(context.Background(), r, res, nil)
	if err != nil {
		return "", err
	}
	return renderGo(req, s)
}

// exportHeaders returns the request's headers as ordered (key, value) pairs in
// sorted order, plus a synthesized Host entry when Go kept a custom Host off
// req.Header (it lives on req.Host).
func exportHeaders(req *http.Request) [][2]string {
	var out [][2]string
	for _, k := range sortedHeaderKeys(req.Header) {
		for _, v := range req.Header[k] {
			out = append(out, [2]string{k, v})
		}
	}
	if req.Host != "" && req.Host != req.URL.Host {
		out = append(out, [2]string{"Host", req.Host})
	}
	return out
}

// jsLit returns s as a JSON/JS/Python string literal (HTML escaping disabled
// for readable snippets). Valid in JavaScript and Python source alike.
func jsLit(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimRight(buf.String(), "\n")
}

func renderFetch(req *http.Request) (string, error) {
	body, err := readBodyBytes(req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "fetch(%s, {\n", jsLit(req.URL.String()))
	fmt.Fprintf(&b, "  method: %s,\n", jsLit(req.Method))
	if hdrs := exportHeaders(req); len(hdrs) > 0 {
		b.WriteString("  headers: {\n")
		for i, kv := range hdrs {
			tail := ","
			if i == len(hdrs)-1 {
				tail = ""
			}
			fmt.Fprintf(&b, "    %s: %s%s\n", jsLit(kv[0]), jsLit(kv[1]), tail)
		}
		b.WriteString("  },\n")
	}
	if len(body) > 0 {
		fmt.Fprintf(&b, "  body: %s,\n", jsLit(string(body)))
	}
	b.WriteString("})\n  .then((r) => r.text())\n  .then(console.log);")
	return b.String(), nil
}

func renderPython(req *http.Request, s model.Settings) (string, error) {
	body, err := readBodyBytes(req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("import requests\n\n")
	b.WriteString("response = requests.request(\n")
	fmt.Fprintf(&b, "    %s,\n", jsLit(req.Method))
	fmt.Fprintf(&b, "    %s,\n", jsLit(req.URL.String()))
	if hdrs := exportHeaders(req); len(hdrs) > 0 {
		b.WriteString("    headers={\n")
		for _, kv := range hdrs {
			fmt.Fprintf(&b, "        %s: %s,\n", jsLit(kv[0]), jsLit(kv[1]))
		}
		b.WriteString("    },\n")
	}
	if len(body) > 0 {
		fmt.Fprintf(&b, "    data=%s,\n", jsLit(string(body)))
	}
	if s.InsecureSkipVerify {
		b.WriteString("    verify=False,\n")
	}
	if !s.FollowRedirects {
		b.WriteString("    allow_redirects=False,\n")
	}
	if s.TimeoutSeconds > 0 {
		fmt.Fprintf(&b, "    timeout=%d,\n", s.TimeoutSeconds)
	}
	b.WriteString(")\nprint(response.text)")
	return b.String(), nil
}

func renderGo(req *http.Request, s model.Settings) (string, error) {
	body, err := readBodyBytes(req)
	if err != nil {
		return "", err
	}
	custom := s.InsecureSkipVerify || s.TimeoutSeconds > 0 || !s.FollowRedirects

	imports := []string{`"fmt"`, `"io"`, `"net/http"`}
	if len(body) > 0 {
		imports = append(imports, `"strings"`)
	}
	if s.InsecureSkipVerify {
		imports = append(imports, `"crypto/tls"`)
	}
	if s.TimeoutSeconds > 0 {
		imports = append(imports, `"time"`)
	}
	sort.Strings(imports)

	var b strings.Builder
	b.WriteString("package main\n\nimport (\n")
	for _, imp := range imports {
		fmt.Fprintf(&b, "\t%s\n", imp)
	}
	b.WriteString(")\n\nfunc main() {\n")

	bodyArg := "nil"
	if len(body) > 0 {
		fmt.Fprintf(&b, "\tbody := strings.NewReader(%s)\n", strconv.Quote(string(body)))
		bodyArg = "body"
	}
	fmt.Fprintf(&b, "\treq, err := http.NewRequest(%s, %s, %s)\n", strconv.Quote(req.Method), strconv.Quote(req.URL.String()), bodyArg)
	b.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	for _, kv := range exportHeaders(req) {
		fmt.Fprintf(&b, "\treq.Header.Add(%s, %s)\n", strconv.Quote(kv[0]), strconv.Quote(kv[1]))
	}

	client := "http.DefaultClient"
	if custom {
		client = "client"
		b.WriteString("\tclient := &http.Client{}\n")
		if s.TimeoutSeconds > 0 {
			fmt.Fprintf(&b, "\tclient.Timeout = %d * time.Second\n", s.TimeoutSeconds)
		}
		if s.InsecureSkipVerify {
			b.WriteString("\tclient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}\n")
		}
		if !s.FollowRedirects {
			b.WriteString("\tclient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }\n")
		}
	}
	fmt.Fprintf(&b, "\tresp, err := %s.Do(req)\n", client)
	b.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	b.WriteString("\tdefer resp.Body.Close()\n")
	b.WriteString("\tout, _ := io.ReadAll(resp.Body)\n")
	b.WriteString("\tfmt.Println(string(out))\n}")
	return b.String(), nil
}
