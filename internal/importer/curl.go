package importer

import (
	"errors"
	"net/url"
	"strings"

	"github.com/idct/helena/internal/model"
)

// FromCurl parses a curl command line into a model.Request. It understands the
// flags that appear in copy-pasted commands: -X/--request, -H/--header,
// -d/--data[-raw|-binary|-ascii]/--data-urlencode, -F/--form, --url + a
// positional URL, -u/--user, -b/--cookie, -A/--user-agent, -e/--referer, and
// -G/--get. Unrecognized flags are skipped (consuming a value for the common
// value-taking ones) so noise like --compressed or -o file doesn't derail the
// parse. Shell quoting (single/double quotes, backslash escapes, and trailing
// "\" line continuations) is handled by the tokenizer.
func FromCurl(command string) (model.Request, error) {
	toks, err := tokenizeShell(command)
	if err != nil {
		return model.Request{}, err
	}
	// Drop a leading "curl" (or a path ending in /curl).
	if len(toks) > 0 {
		if t := toks[0]; t == "curl" || strings.HasSuffix(t, "/curl") {
			toks = toks[1:]
		}
	}

	req := model.Request{Body: model.Body{Type: model.BodyNone}}
	var (
		methodSet   bool
		urlStr      string
		dataParts   []string
		hasData     bool
		formParts   []model.KeyValue
		getMode     bool
		contentType string
	)

	// value reads the value for a flag at toks[i]: an inline "--flag=v", an
	// attached short "-Xv", or the next token. It advances i past a consumed
	// next token. Returns false when a value was required but missing.
	value := func(i *int, flag, inline string, attached bool) (string, bool) {
		if inline != "" || attached {
			return inline, true
		}
		if *i+1 >= len(toks) {
			return "", false
		}
		*i++
		return toks[*i], true
	}

	for i := 0; i < len(toks); i++ {
		raw := toks[i]
		if raw == "" {
			continue
		}
		// Positional argument → URL (first one wins).
		if !strings.HasPrefix(raw, "-") || raw == "-" {
			if urlStr == "" {
				urlStr = raw
			}
			continue
		}

		// Normalize: split "--flag=value" and detached short "-Hvalue".
		flag, inline, attached := splitFlag(raw)

		switch flag {
		case "-X", "--request":
			if v, ok := value(&i, flag, inline, attached); ok {
				req.Method = model.Method(strings.ToUpper(strings.TrimSpace(v)))
				methodSet = true
			}
		case "-H", "--header":
			if v, ok := value(&i, flag, inline, attached); ok {
				if k, val, found := strings.Cut(v, ":"); found {
					k, val = strings.TrimSpace(k), strings.TrimSpace(val)
					if k != "" {
						req.Headers = append(req.Headers, model.KeyValue{Enabled: true, Key: k, Value: val})
						if strings.EqualFold(k, "Content-Type") {
							contentType = val
						}
					}
				}
			}
		case "-A", "--user-agent":
			if v, ok := value(&i, flag, inline, attached); ok {
				req.Headers = append(req.Headers, model.KeyValue{Enabled: true, Key: "User-Agent", Value: v})
			}
		case "-e", "--referer":
			if v, ok := value(&i, flag, inline, attached); ok {
				req.Headers = append(req.Headers, model.KeyValue{Enabled: true, Key: "Referer", Value: v})
			}
		case "-b", "--cookie":
			// A cookie arg is "name=value;…"; the bare-filename form is rare in
			// pasted commands, so treat the value as a Cookie header verbatim.
			if v, ok := value(&i, flag, inline, attached); ok {
				req.Headers = append(req.Headers, model.KeyValue{Enabled: true, Key: "Cookie", Value: v})
			}
		case "-u", "--user":
			if v, ok := value(&i, flag, inline, attached); ok {
				user, pass, _ := strings.Cut(v, ":")
				req.Auth = model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: user, Password: pass}}
			}
		case "--url":
			if v, ok := value(&i, flag, inline, attached); ok && urlStr == "" {
				urlStr = v
			}
		case "-d", "--data", "--data-raw", "--data-ascii", "--data-binary", "--data-urlencode":
			if v, ok := value(&i, flag, inline, attached); ok {
				if flag == "--data-urlencode" {
					v = urlencodeData(v) // curl percent-encodes --data-urlencode
				}
				dataParts = append(dataParts, v)
				hasData = true
			}
		case "-F", "--form":
			if v, ok := value(&i, flag, inline, attached); ok {
				k, val, _ := strings.Cut(v, "=")
				if k = strings.TrimSpace(k); k != "" {
					formParts = append(formParts, model.KeyValue{Enabled: true, Key: k, Value: val})
				}
			}
		case "-G", "--get":
			getMode = true
		default:
			// Unknown flag: consume a value for the common value-taking ones so the
			// URL/data parse isn't thrown off; otherwise treat as a boolean.
			if !attached && inline == "" && curlValueFlags[flag] {
				i++ // skip its value
			}
		}
	}

	if urlStr == "" {
		return model.Request{}, errors.New("curl: no URL found in command")
	}

	switch {
	case len(formParts) > 0:
		req.Body = model.Body{Type: model.BodyMultipart, Form: formParts}
		// Drop any pasted multipart Content-Type: its boundary won't match the
		// one the send path generates, so let httpclient supply a fresh header.
		req.Headers = dropMultipartContentType(req.Headers)
		if !methodSet {
			req.Method = model.POST
		}
	case hasData:
		data := strings.Join(dataParts, "&")
		if getMode {
			urlStr = appendQuery(urlStr, data)
			if !methodSet {
				req.Method = model.GET
			}
		} else {
			req.Body = bodyFromData(data, contentType)
			if !methodSet {
				req.Method = model.POST
			}
		}
	}
	if req.Method == "" {
		req.Method = model.GET
	}
	req.URL = urlStr
	req.Name = curlName(req.Method, urlStr)
	return req, nil
}

// urlencodeData percent-encodes a --data-urlencode value the way curl does:
// "name=content" encodes only content; "=content" or a bare value encodes the
// whole thing.
func urlencodeData(s string) string {
	if name, content, found := strings.Cut(s, "="); found && name != "" {
		return name + "=" + url.QueryEscape(content)
	}
	return url.QueryEscape(strings.TrimPrefix(s, "="))
}

// dropMultipartContentType removes a Content-Type: multipart/form-data header so
// the send path can generate a fresh boundary that matches the body.
func dropMultipartContentType(headers []model.KeyValue) []model.KeyValue {
	out := headers[:0]
	for _, h := range headers {
		if strings.EqualFold(h.Key, "Content-Type") && strings.Contains(strings.ToLower(h.Value), "multipart/form-data") {
			continue
		}
		out = append(out, h)
	}
	return out
}

// splitFlag breaks a flag token into (flag, inlineValue, attachedShort). A long
// "--flag=value" yields ("--flag", "value", false); a short "-Hvalue" yields
// ("-H", "value", true); anything else yields (token, "", false).
func splitFlag(tok string) (flag, inline string, attached bool) {
	if strings.HasPrefix(tok, "--") {
		if k, v, found := strings.Cut(tok, "="); found {
			return k, v, false
		}
		return tok, "", false
	}
	// short flag: -X may carry an attached value (-XPOST, -d'data', -Hx:y)
	if len(tok) > 2 && curlValueShorts[tok[:2]] {
		return tok[:2], tok[2:], true
	}
	return tok, "", false
}

// curlValueShorts are the single-dash flags that take a value (and so may carry
// it attached, e.g. -XPOST).
var curlValueShorts = map[string]bool{
	"-X": true, "-H": true, "-d": true, "-F": true, "-u": true,
	"-A": true, "-e": true, "-b": true, "-o": true, "-m": true,
	"-x": true, "-E": true, "-T": true, "-w": true,
}

// curlValueFlags are flags Helena ignores but which consume a following value,
// so skipping them doesn't swallow the URL or data.
var curlValueFlags = map[string]bool{
	"-o": true, "--output": true, "-m": true, "--max-time": true,
	"--connect-timeout": true, "-w": true, "--write-out": true,
	"-x": true, "--proxy": true, "-E": true, "--cert": true,
	"--key": true, "--cacert": true, "-T": true, "--upload-file": true,
	"--retry": true, "--resolve": true, "--limit-rate": true,
}

// bodyFromData maps a -d data string + optional Content-Type to a model.Body.
// With no Content-Type it sniffs JSON ({…}/[…]) and form (key=value&…) shapes.
func bodyFromData(data, contentType string) model.Body {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		return model.Body{Type: model.BodyJSON, Content: data}
	case strings.Contains(ct, "xml"):
		return model.Body{Type: model.BodyXML, Content: data}
	case strings.Contains(ct, "x-www-form-urlencoded"):
		return model.Body{Type: model.BodyForm, Form: parseFormData(data)}
	case ct != "":
		return model.Body{Type: model.BodyText, Content: data}
	}
	trimmed := strings.TrimSpace(data)
	switch {
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		return model.Body{Type: model.BodyJSON, Content: data}
	case looksLikeForm(data):
		return model.Body{Type: model.BodyForm, Form: parseFormData(data)}
	default:
		return model.Body{Type: model.BodyText, Content: data}
	}
}

// looksLikeForm reports whether data reads like an x-www-form-urlencoded body
// (has "=" and no whitespace, which JSON/text bodies would carry).
func looksLikeForm(data string) bool {
	return strings.Contains(data, "=") && !strings.ContainsAny(data, " \t\n")
}

// parseFormData splits "a=1&b=2" into ordered KeyValue rows, percent-decoding.
func parseFormData(data string) []model.KeyValue {
	var out []model.KeyValue
	for _, part := range strings.Split(data, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		if k == "" {
			continue
		}
		out = append(out, model.KeyValue{Enabled: true, Key: k, Value: v})
	}
	return out
}

// appendQuery joins extra onto url's query string (used by -G/--get).
func appendQuery(rawURL, extra string) string {
	if extra == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + extra
}

// curlName derives a short request name from the method + URL.
func curlName(method model.Method, rawURL string) string {
	label := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		label = u.Host + u.Path
		label = strings.TrimSuffix(label, "/")
	}
	if label == "" {
		label = rawURL
	}
	return string(method) + " " + label
}

// tokenizeShell splits a shell-style command into tokens, honouring single
// quotes (literal), double quotes (with \" \\ \$ \` escapes), backslash escapes
// outside quotes, and trailing-backslash line continuations.
func tokenizeShell(s string) ([]string, error) {
	var (
		toks    []string
		cur     strings.Builder
		started bool
	)
	flush := func() {
		if started {
			toks = append(toks, cur.String())
			cur.Reset()
			started = false
		}
	}
	const (
		normal = iota
		single
		double
	)
	state := normal
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {
		case normal:
			switch c {
			case ' ', '\t', '\n', '\r':
				flush()
			case '\'':
				started = true
				state = single
			case '"':
				started = true
				state = double
			case '\\':
				if i+1 < len(s) {
					i++
					if s[i] == '\n' { // line continuation
						continue
					}
					if s[i] == '\r' {
						if i+1 < len(s) && s[i+1] == '\n' {
							i++
						}
						continue
					}
					cur.WriteByte(s[i])
					started = true
				}
			default:
				cur.WriteByte(c)
				started = true
			}
		case single:
			if c == '\'' {
				state = normal
			} else {
				cur.WriteByte(c)
			}
		case double:
			switch c {
			case '"':
				state = normal
			case '\\':
				if i+1 < len(s) {
					n := s[i+1]
					switch n {
					case '"', '\\', '$', '`':
						cur.WriteByte(n)
						i++
					case '\n':
						i++ // line continuation inside quotes
					default:
						cur.WriteByte('\\')
					}
				} else {
					cur.WriteByte('\\')
				}
			default:
				cur.WriteByte(c)
			}
		}
	}
	if state != normal {
		return nil, errors.New("curl: unterminated quote in command")
	}
	flush()
	return toks, nil
}
