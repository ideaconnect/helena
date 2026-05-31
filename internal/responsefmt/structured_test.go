package responsefmt

import "testing"

// childByLabel returns the first child of n whose Label matches, or nil.
func childByLabel(n *Node, label string) *Node {
	for _, c := range n.Children {
		if c.Label == label {
			return c
		}
	}
	return nil
}

// TestParseJSONBuildsOrderedTree verifies object key order is preserved and
// each scalar gets the right value + color kind.
func TestParseJSONBuildsOrderedTree(t *testing.T) {
	root, err := ParseJSON([]byte(`{"name":"helena","n":42,"ok":true,"x":null}`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if root.Value != "{4}" || root.Kind != TokenPunct {
		t.Errorf("root summary = %q kind %d, want {4}/punct", root.Value, root.Kind)
	}
	wantOrder := []string{`"name"`, `"n"`, `"ok"`, `"x"`}
	for i, w := range wantOrder {
		if root.Children[i].Label != w {
			t.Fatalf("child %d label = %q, want %q", i, root.Children[i].Label, w)
		}
	}
	cases := []struct {
		label, value string
		kind         TokenKind
	}{
		{`"name"`, `"helena"`, TokenString},
		{`"n"`, `42`, TokenNumber},
		{`"ok"`, `true`, TokenBool},
		{`"x"`, `null`, TokenNull},
	}
	for _, c := range cases {
		got := childByLabel(root, c.label)
		if got == nil || got.Value != c.value || got.Kind != c.kind {
			t.Errorf("child %s = %+v, want value %q kind %d", c.label, got, c.value, c.kind)
		}
		if got.LabelKind != TokenKey {
			t.Errorf("child %s label kind = %d, want key", c.label, got.LabelKind)
		}
	}
}

// TestParseJSONNestedAndArrays verifies arrays index their children, nested
// containers carry summaries, and IDs are unique paths.
func TestParseJSONNestedAndArrays(t *testing.T) {
	root, err := ParseJSON([]byte(`{"a":[10,{"b":1}]}`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	arr := childByLabel(root, `"a"`)
	if arr == nil || arr.Value != "[2]" {
		t.Fatalf("array node = %+v, want [2]", arr)
	}
	if arr.Children[0].Label != "[0]" || arr.Children[0].LabelKind != TokenPunct {
		t.Errorf("array index label = %q kind %d", arr.Children[0].Label, arr.Children[0].LabelKind)
	}
	obj := arr.Children[1]
	if obj.Value != "{1}" || obj.ID != "$/0/1" {
		t.Errorf("nested object = value %q id %q, want {1}/$/0/1", obj.Value, obj.ID)
	}
}

// TestParseJSONErrors verifies invalid JSON and trailing data are rejected.
func TestParseJSONErrors(t *testing.T) {
	if _, err := ParseJSON([]byte(`{`)); err == nil {
		t.Error("expected error for truncated JSON")
	}
	if _, err := ParseJSON([]byte(`{} junk`)); err == nil {
		t.Error("expected error for trailing data")
	}
}

// TestParseXMLBuildsTree verifies nested elements, attributes in the label,
// and leaf text becoming a string-colored value.
func TestParseXMLBuildsTree(t *testing.T) {
	root, err := ParseXML([]byte(`<root id="1"><item>hi</item><empty/></root>`))
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}
	if root.Label != `root id="1"` || root.LabelKind != TokenTag {
		t.Errorf("root label = %q kind %d", root.Label, root.LabelKind)
	}
	if len(root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(root.Children))
	}
	item := root.Children[0]
	if item.Label != "item" || item.Value != "hi" || item.Kind != TokenString {
		t.Errorf("item = %+v, want label item value hi string", item)
	}
	if empty := root.Children[1]; empty.Value != "" || len(empty.Children) != 0 {
		t.Errorf("empty element = %+v, want no value/children", empty)
	}
}

// TestParseXMLErrors verifies malformed XML and element-free input are rejected.
func TestParseXMLErrors(t *testing.T) {
	if _, err := ParseXML([]byte(`<root><unclosed></root>`)); err == nil {
		t.Error("expected error for mismatched tags")
	}
	if _, err := ParseXML([]byte(`   `)); err == nil {
		t.Error("expected error for element-free input")
	}
}
