package responsefmt

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Node is one row of the Structured response tree. A container node (a JSON
// object/array or an XML element with children) carries Children and a count
// summary Value such as "{3}" or "[5]"; a leaf carries a scalar Value colored
// by Kind. Label is the left-hand caption — a quoted JSON key, an array index
// like "[0]", or an XML element name with its attributes — colored by
// LabelKind. ID is a stable, unique path used as the tree node identifier.
type Node struct {
	ID        string
	Label     string
	LabelKind TokenKind
	Value     string
	Kind      TokenKind
	Children  []*Node
}

// ParseJSON builds an ordered Structured tree from a JSON document, preserving
// object key order (unlike a map decode). It errors on invalid JSON or on
// trailing data after the first value.
func ParseJSON(body []byte) (*Node, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	root, err := buildJSONValue(dec, "", TokenPlain, "$")
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return root, nil
}

// buildJSONValue consumes one JSON value (and its children) from dec, tagging
// the resulting node with the caller-supplied label/labelKind/id.
func buildJSONValue(dec *json.Decoder, label string, labelKind TokenKind, id string) (*Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	node := &Node{ID: id, Label: label, LabelKind: labelKind}
	delim, ok := tok.(json.Delim)
	if !ok {
		node.Value, node.Kind = scalarJSON(tok)
		return node, nil
	}
	switch delim {
	case '{':
		idx := 0
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key := keyTok.(string)
			child, err := buildJSONValue(dec, strconv.Quote(key), TokenKey, id+"/"+strconv.Itoa(idx))
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
			idx++
		}
		node.Value = fmt.Sprintf("{%d}", len(node.Children))
	case '[':
		idx := 0
		for dec.More() {
			child, err := buildJSONValue(dec, "["+strconv.Itoa(idx)+"]", TokenPunct, id+"/"+strconv.Itoa(idx))
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
			idx++
		}
		node.Value = fmt.Sprintf("[%d]", len(node.Children))
	}
	node.Kind = TokenPunct
	if _, err := dec.Token(); err != nil { // closing delimiter
		return nil, err
	}
	return node, nil
}

// scalarJSON renders a JSON scalar token as its display string and color kind.
func scalarJSON(tok json.Token) (string, TokenKind) {
	switch v := tok.(type) {
	case string:
		return strconv.Quote(v), TokenString
	case json.Number:
		return v.String(), TokenNumber
	case bool:
		return strconv.FormatBool(v), TokenBool
	case nil:
		return "null", TokenNull
	default:
		return fmt.Sprint(v), TokenPlain
	}
}

// ParseXML builds a Structured tree from an XML document. Each element becomes
// a node labeled with its tag name and attributes; an element whose only
// content is text carries that text as its (string-colored) Value. It errors
// on malformed XML or a document with no elements.
func ParseXML(body []byte) (*Node, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	root := &Node{}
	stack := []*Node{root}
	text := map[*Node]*strings.Builder{}
	counter := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			counter++
			node := &Node{ID: "x" + strconv.Itoa(counter), Label: xmlStartLabel(t), LabelKind: TokenTag, Kind: TokenPunct}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, node)
			stack = append(stack, node)
		case xml.EndElement:
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(node.Children) == 0 {
				if b := text[node]; b != nil {
					if s := strings.TrimSpace(b.String()); s != "" {
						node.Value, node.Kind = s, TokenString
					}
				}
			}
		case xml.CharData:
			node := stack[len(stack)-1]
			if node == root {
				continue
			}
			b := text[node]
			if b == nil {
				b = &strings.Builder{}
				text[node] = b
			}
			b.Write(t)
		}
	}
	if len(root.Children) == 0 {
		return nil, fmt.Errorf("no XML elements")
	}
	return root.Children[0], nil
}

// xmlStartLabel renders an element's display label: its local name, with any
// attributes appended as ` name="value"` pairs.
func xmlStartLabel(t xml.StartElement) string {
	if len(t.Attr) == 0 {
		return t.Name.Local
	}
	var b strings.Builder
	b.WriteString(t.Name.Local)
	for _, a := range t.Attr {
		b.WriteString(" ")
		b.WriteString(a.Name.Local)
		b.WriteString("=")
		b.WriteString(strconv.Quote(a.Value))
	}
	return b.String()
}
