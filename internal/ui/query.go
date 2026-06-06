package ui

import (
	"net/url"
	"strings"

	"github.com/idct/helena/internal/model"
)

// Two-way "Query" sync helpers. The Query tab and the URL field are two views of
// the same data: the URL field shows base + query, while currentRequest keeps
// the bare base in URL and the structured pairs (incl. disabled ones) in Params.
// Params stays the source of truth the send path merges, so the backend is
// unchanged — these helpers only translate between the two views in the UI.

// splitURLQuery splits a URL into the part before the first "?" (base) and the
// query string after it. A URL with no "?" yields an empty query.
func splitURLQuery(raw string) (base, query string) {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

// parseQueryParams turns a raw query string ("a=1&b=hi") into enabled KeyValue
// rows, percent-decoding keys and values (leaving {{vars}}, which carry no
// escapes, untouched). Rows with an empty key are dropped.
func parseQueryParams(query string) []model.KeyValue {
	if query == "" {
		return nil
	}
	var out []model.KeyValue
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		key, val, _ := strings.Cut(part, "=")
		key, val = queryUnescape(key), queryUnescape(val)
		if key == "" {
			continue
		}
		out = append(out, model.KeyValue{Enabled: true, Key: key, Value: val})
	}
	return out
}

// buildQueryString renders the enabled, non-empty-key params back into a query
// string, percent-encoding while leaving {{var}} placeholders intact.
func buildQueryString(params []model.KeyValue) string {
	var parts []string
	for _, p := range params {
		if !p.Enabled || p.Key == "" {
			continue
		}
		parts = append(parts, encodeQueryComponent(p.Key)+"="+encodeQueryComponent(p.Value))
	}
	return strings.Join(parts, "&")
}

// displayURL combines a query-less base with the query rebuilt from params.
func displayURL(base string, params []model.KeyValue) string {
	if q := buildQueryString(params); q != "" {
		return base + "?" + q
	}
	return base
}

// mergeQueryFromURL replaces the enabled query rows with the ones parsed from
// the URL while preserving any disabled rows (which a URL can't express), so
// toggling a row off and then editing the URL doesn't silently lose it.
func mergeQueryFromURL(existing, fromURL []model.KeyValue) []model.KeyValue {
	out := append([]model.KeyValue(nil), fromURL...)
	for _, p := range existing {
		if !p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

// encodeQueryComponent percent-encodes s for a URL query but keeps {{var}}
// placeholders verbatim, so they still resolve at send time.
func encodeQueryComponent(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(url.QueryEscape(s))
			return b.String()
		}
		b.WriteString(url.QueryEscape(s[:i]))
		rest := s[i:]
		j := strings.Index(rest, "}}")
		if j < 0 {
			b.WriteString(url.QueryEscape(rest))
			return b.String()
		}
		b.WriteString(rest[:j+2]) // {{...}} kept verbatim
		s = rest[j+2:]
	}
}

// queryUnescape decodes a percent-encoded query component, falling back to the
// raw text on malformed input (e.g. a stray % inside a {{var}}).
func queryUnescape(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}
