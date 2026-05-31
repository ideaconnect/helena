package responsefmt

import (
	"strings"
	"testing"
)

// concat reassembles the original text from a token slice so tests can assert
// the lossless concatenation-equals-input invariant.
func concat(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.Text)
	}
	return b.String()
}

// kindOf returns the kind of the first token whose text matches want, or -1.
func kindOf(toks []Token, want string) TokenKind {
	for _, t := range toks {
		if t.Text == want {
			return t.Kind
		}
	}
	return -1
}

// TestHighlightJSONIsLossless verifies the tokens concatenate back to the input.
func TestHighlightJSONIsLossless(t *testing.T) {
	in := "{\n  \"name\": \"helena\",\n  \"n\": -12.5e3,\n  \"ok\": true,\n  \"x\": null,\n  \"a\": [1, 2]\n}"
	if got := concat(HighlightJSON(in)); got != in {
		t.Fatalf("lossy: got %q want %q", got, in)
	}
}

// TestHighlightJSONClassifiesTokens verifies keys, values, and literals get
// the right kinds — notably a quoted name before ':' is a key, not a string.
func TestHighlightJSONClassifiesTokens(t *testing.T) {
	toks := HighlightJSON(`{"name": "helena", "n": -12.5, "ok": true, "x": null}`)
	cases := []struct {
		text string
		want TokenKind
	}{
		{`"name"`, TokenKey},
		{`"helena"`, TokenString},
		{`-12.5`, TokenNumber},
		{`true`, TokenBool},
		{`null`, TokenNull},
		{`{`, TokenPunct},
		{`:`, TokenPunct},
	}
	for _, c := range cases {
		if got := kindOf(toks, c.text); got != c.want {
			t.Errorf("kind(%s) = %d, want %d", c.text, got, c.want)
		}
	}
}

// TestHighlightJSONHandlesEscapedQuote verifies a backslash-escaped quote
// inside a string does not terminate the string token early.
func TestHighlightJSONHandlesEscapedQuote(t *testing.T) {
	in := `{"k": "a\"b"}`
	toks := HighlightJSON(in)
	if concat(toks) != in {
		t.Fatalf("lossy on escaped quote: %q", concat(toks))
	}
	if kindOf(toks, `"a\"b"`) != TokenString {
		t.Errorf("escaped-quote string not tokenized as one string value")
	}
}

// TestHighlightJSONUnterminatedString verifies a truncated string still yields
// a lossless token stream rather than panicking on an out-of-range slice.
func TestHighlightJSONUnterminatedString(t *testing.T) {
	in := `{"k": "unterminated`
	if got := concat(HighlightJSON(in)); got != in {
		t.Fatalf("lossy on unterminated string: %q", got)
	}
}

// TestHighlightXMLIsLossless verifies XML tokens concatenate back to the input
// across tags, attributes, text, comments, CDATA, and declarations.
func TestHighlightXMLIsLossless(t *testing.T) {
	in := "<?xml version=\"1.0\"?>\n<!-- note -->\n<root id=\"1\">\n  <item>hi</item>\n  <empty/>\n  <c><![CDATA[ raw <x> ]]></c>\n</root>"
	if got := concat(HighlightXML(in)); got != in {
		t.Fatalf("lossy: got %q want %q", got, in)
	}
}

// TestHighlightXMLClassifiesTokens verifies element names, attribute names and
// values, text, and comment/CDATA/PI spans get the right kinds.
func TestHighlightXMLClassifiesTokens(t *testing.T) {
	toks := HighlightXML(`<?pi?><!-- c --><root id="1">hi<![CDATA[x]]></root>`)
	cases := []struct {
		text string
		want TokenKind
	}{
		{`root`, TokenTag},
		{`id`, TokenAttr},
		{`"1"`, TokenString},
		{`hi`, TokenString},
		{`<?pi?>`, TokenComment},
		{`<!-- c -->`, TokenComment},
		{`<![CDATA[x]]>`, TokenComment},
		{`<`, TokenPunct},
		{`=`, TokenPunct},
	}
	for _, c := range cases {
		if got := kindOf(toks, c.text); got != c.want {
			t.Errorf("kind(%s) = %d, want %d", c.text, got, c.want)
		}
	}
}

// TestHighlightXMLSelfClosingAndDoctype verifies a self-closing tag and a
// <!DOCTYPE> declaration tokenize losslessly.
func TestHighlightXMLSelfClosingAndDoctype(t *testing.T) {
	in := `<!DOCTYPE html><br/><x a='v'/>`
	toks := HighlightXML(in)
	if concat(toks) != in {
		t.Fatalf("lossy: %q", concat(toks))
	}
	if kindOf(toks, `<!DOCTYPE html>`) != TokenComment {
		t.Errorf("DOCTYPE not classified as comment/declaration")
	}
	if kindOf(toks, `'v'`) != TokenString {
		t.Errorf("single-quoted attr value not a string")
	}
}

// TestHighlightXMLTruncatedSpans verifies unterminated comment/CDATA/PI spans
// run to end-of-input without panicking.
func TestHighlightXMLTruncatedSpans(t *testing.T) {
	for _, in := range []string{"<!-- open", "<![CDATA[ open", "<?pi open", "<!decl open", "<tag attr"} {
		if got := concat(HighlightXML(in)); got != in {
			t.Errorf("lossy on %q: got %q", in, got)
		}
	}
}
