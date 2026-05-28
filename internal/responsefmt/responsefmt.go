// Package responsefmt formats HTTP response bodies and headers for display:
// pretty-printers for JSON and XML, a sortable header serializer, and small
// human-readable helpers for response size and duration.
package responsefmt

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// PrettyJSON returns body re-indented with two spaces, or an error if body is
// not valid JSON.
func PrettyJSON(body []byte) (string, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return "", err
	}
	return out.String(), nil
}

// PrettyXML returns body re-indented with two spaces. Whitespace-only text
// nodes from the input are stripped so the encoder can re-format cleanly.
func PrettyXML(body []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	enc.Indent("", "  ")
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if cd, ok := tok.(xml.CharData); ok && len(bytes.TrimSpace(cd)) == 0 {
			continue
		}
		if err := enc.EncodeToken(tok); err != nil {
			return "", err
		}
	}
	if err := enc.Flush(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// IsJSON reports whether contentType denotes a JSON payload.
func IsJSON(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json")
}

// IsXML reports whether contentType denotes an XML payload (including SOAP).
func IsXML(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "xml")
}

// FormatHeaders renders headers as sorted "Key: value" lines, one per value.
func FormatHeaders(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// HumanSize formats a byte count using binary units (B / KB / MB / GB).
func HumanSize(n int64) string {
	const k = 1024.0
	switch {
	case n < int64(k):
		return fmt.Sprintf("%d B", n)
	case n < int64(k*k):
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	case n < int64(k*k*k):
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
	}
}

// HumanDuration formats a duration: "N ms" below 1s, "X.XX s" up to a minute,
// "Mm Ss" beyond.
func HumanDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm %ds", m, s)
}
