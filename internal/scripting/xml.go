package scripting

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
)

// xmlMaxDepth caps recursion in readXMLElement so a pathologically
// deep response body can't blow the goroutine stack. 256 is generous
// for real-world XML (SOAP envelopes rarely exceed 20) and far below
// the depth needed to overflow Go's default stack.
const xmlMaxDepth = 256

// errXMLTooDeep is the sentinel returned when xmlMaxDepth is exceeded.
// tryParseXML treats every readXMLElement error as "not XML" so callers
// just see response.xml stay undefined.
var errXMLTooDeep = errors.New("xml: nesting too deep")

// tryParseXML converts body into a JS-friendly nested map. Each element
// is an object whose attribute set lives under "$", text content under
// "_", and child elements under their tag name. A tag that appears more
// than once at the same level coalesces into an array; a tag that
// appears exactly once stays a plain object (the xml2js
// explicitArray:false convention — verbose paths are the common case in
// API responses).
//
// Returns (nil, false) when body doesn't parse as XML or exceeds
// xmlMaxDepth nesting.
func tryParseXML(body []byte) (interface{}, bool) {
	s := strings.TrimLeft(string(body), " \t\r\n")
	if !strings.HasPrefix(s, "<") {
		return nil, false
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		elem, err := readXMLElement(dec, start, 0)
		if err != nil {
			return nil, false
		}
		return map[string]interface{}{start.Name.Local: elem}, true
	}
}

// readXMLElement consumes tokens until it sees the matching end tag for
// start. The caller is responsible for advancing the decoder to start
// in the first place. depth is the recursion level (0 at the root);
// exceeding xmlMaxDepth returns errXMLTooDeep so a malformed or
// pathological body can't drive an unbounded stack walk.
func readXMLElement(dec *xml.Decoder, start xml.StartElement, depth int) (map[string]interface{}, error) {
	if depth > xmlMaxDepth {
		return nil, errXMLTooDeep
	}
	out := map[string]interface{}{}
	if len(start.Attr) > 0 {
		attrs := map[string]string{}
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		out["$"] = attrs
	}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := readXMLElement(dec, t, depth+1)
			if err != nil {
				return nil, err
			}
			name := t.Name.Local
			if existing, ok := out[name]; ok {
				if arr, ok := existing.([]interface{}); ok {
					out[name] = append(arr, child)
				} else {
					out[name] = []interface{}{existing, child}
				}
			} else {
				out[name] = child
			}
		case xml.CharData:
			text.Write([]byte(t))
		case xml.EndElement:
			if s := strings.TrimSpace(text.String()); s != "" {
				out["_"] = s
			}
			return out, nil
		}
	}
}
