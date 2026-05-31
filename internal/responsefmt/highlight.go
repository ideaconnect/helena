package responsefmt

import "strings"

// TokenKind classifies a highlighted run so the UI can pick a display color.
// The same enum colors both the Pretty view (via HighlightJSON / HighlightXML)
// and the Structured tree's labels and values, so a key reads the same blue
// in either view.
type TokenKind int

const (
	// TokenPlain is structural filler (whitespace, indentation) with no
	// special color; the UI renders it in the theme's default foreground.
	TokenPlain TokenKind = iota
	// TokenPunct is JSON {}[],: or XML angle brackets, slashes, and '='.
	TokenPunct
	// TokenKey is a JSON object key — the quoted name before its ':'.
	TokenKey
	// TokenString is a JSON string value, an XML attribute value, or XML text.
	TokenString
	// TokenNumber is a JSON number.
	TokenNumber
	// TokenBool is a JSON true / false literal.
	TokenBool
	// TokenNull is a JSON null literal.
	TokenNull
	// TokenTag is an XML element name.
	TokenTag
	// TokenAttr is an XML attribute name.
	TokenAttr
	// TokenComment is an XML comment, CDATA section, processing instruction,
	// or declaration (<!-- -->, <![CDATA[ ]]>, <? ?>, <! >).
	TokenComment
)

// Token is one colored run of text in document order. Concatenating every
// Token.Text reproduces the input verbatim, so the UI renders the whole
// document by walking the slice and coloring each run by its Kind.
type Token struct {
	Text string
	Kind TokenKind
}

// HighlightJSON tokenizes already-valid JSON text for colored display. It does
// not validate — callers tokenize the output of PrettyJSON, which has already
// proven the document parses. Anything unexpected is emitted as TokenPlain so
// the concatenation-equals-input invariant holds even on odd input.
func HighlightJSON(s string) []Token {
	var toks []Token
	emit := func(t string, k TokenKind) {
		if t != "" {
			toks = append(toks, Token{Text: t, Kind: k})
		}
	}
	n := len(s)
	for i := 0; i < n; {
		c := s[i]
		switch {
		case isJSONSpace(c):
			j := i
			for j < n && isJSONSpace(s[j]) {
				j++
			}
			emit(s[i:j], TokenPlain)
			i = j
		case c == '{' || c == '}' || c == '[' || c == ']' || c == ',' || c == ':':
			emit(s[i:i+1], TokenPunct)
			i++
		case c == '"':
			j := scanJSONString(s, i)
			kind := TokenString
			if k := skipJSONSpace(s, j); k < n && s[k] == ':' {
				kind = TokenKey
			}
			emit(s[i:j], kind)
			i = j
		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < n && isJSONNumberByte(s[j]) {
				j++
			}
			emit(s[i:j], TokenNumber)
			i = j
		case strings.HasPrefix(s[i:], "true"):
			emit("true", TokenBool)
			i += len("true")
		case strings.HasPrefix(s[i:], "false"):
			emit("false", TokenBool)
			i += len("false")
		case strings.HasPrefix(s[i:], "null"):
			emit("null", TokenNull)
			i += len("null")
		default:
			emit(s[i:i+1], TokenPlain)
			i++
		}
	}
	return toks
}

// scanJSONString returns the index just past the closing quote of the JSON
// string starting at the opening quote s[start]. Backslash escapes are
// skipped; an unterminated string runs to the end of input.
func scanJSONString(s string, start int) int {
	n := len(s)
	j := start + 1
	for j < n {
		switch s[j] {
		case '\\':
			j += 2
		case '"':
			return j + 1
		default:
			j++
		}
	}
	if j > n {
		j = n
	}
	return j
}

func skipJSONSpace(s string, i int) int {
	for i < len(s) && isJSONSpace(s[i]) {
		i++
	}
	return i
}

func isJSONSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isJSONNumberByte(c byte) bool {
	return (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E'
}

// HighlightXML tokenizes already-valid XML text for colored display, with the
// same concatenation-equals-input invariant as HighlightJSON. Tags, attribute
// names and values, text, and comment/CDATA/PI/declaration spans each get
// their own kind.
func HighlightXML(s string) []Token {
	var toks []Token
	emit := func(t string, k TokenKind) {
		if t != "" {
			toks = append(toks, Token{Text: t, Kind: k})
		}
	}
	n := len(s)
	for i := 0; i < n; {
		if s[i] != '<' {
			j := i
			for j < n && s[j] != '<' {
				j++
			}
			run := s[i:j]
			if strings.TrimSpace(run) == "" {
				emit(run, TokenPlain)
			} else {
				emit(run, TokenString)
			}
			i = j
			continue
		}
		// At '<'. Spans that run to a multi-byte terminator are colored whole.
		if end, ok := spanUntil(s, i, "<!--", "-->"); ok {
			emit(s[i:end], TokenComment)
			i = end
			continue
		}
		if end, ok := spanUntil(s, i, "<![CDATA[", "]]>"); ok {
			emit(s[i:end], TokenComment)
			i = end
			continue
		}
		if end, ok := spanUntil(s, i, "<?", "?>"); ok {
			emit(s[i:end], TokenComment)
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "<!") {
			end := n
			if gt := strings.IndexByte(s[i:], '>'); gt >= 0 {
				end = i + gt + 1
			}
			emit(s[i:end], TokenComment)
			i = end
			continue
		}
		end := n
		if gt := strings.IndexByte(s[i:], '>'); gt >= 0 {
			end = i + gt + 1
		}
		highlightXMLTag(s[i:end], emit)
		i = end
	}
	return toks
}

// spanUntil reports whether s at start opens with prefix; if so it returns the
// index just past the next occurrence of suffix (or end-of-input when the
// document is truncated mid-span).
func spanUntil(s string, start int, prefix, suffix string) (int, bool) {
	if !strings.HasPrefix(s[start:], prefix) {
		return 0, false
	}
	rest := start + len(prefix)
	if idx := strings.Index(s[rest:], suffix); idx >= 0 {
		return rest + idx + len(suffix), true
	}
	return len(s), true
}

// highlightXMLTag tokenizes one element tag — `<name a="b"/>` or `</name>` —
// emitting the brackets/slashes/'=' as punctuation, the element name as a tag,
// attribute names as attrs, and quoted attribute values as strings.
func highlightXMLTag(tag string, emit func(string, TokenKind)) {
	n := len(tag)
	i := 0
	if i < n && tag[i] == '<' {
		emit("<", TokenPunct)
		i++
	}
	if i < n && tag[i] == '/' {
		emit("/", TokenPunct)
		i++
	}
	j := i
	for j < n && !isXMLSpace(tag[j]) && tag[j] != '>' && tag[j] != '/' {
		j++
	}
	emit(tag[i:j], TokenTag)
	i = j
	for i < n {
		c := tag[i]
		switch {
		case isXMLSpace(c):
			k := i
			for k < n && isXMLSpace(tag[k]) {
				k++
			}
			emit(tag[i:k], TokenPlain)
			i = k
		case c == '>' || c == '/' || c == '=':
			emit(tag[i:i+1], TokenPunct)
			i++
		case c == '"' || c == '\'':
			k := i + 1
			for k < n && tag[k] != c {
				k++
			}
			if k < n {
				k++
			}
			emit(tag[i:k], TokenString)
			i = k
		default:
			k := i
			for k < n && !isXMLSpace(tag[k]) && tag[k] != '=' && tag[k] != '>' && tag[k] != '/' {
				k++
			}
			emit(tag[i:k], TokenAttr)
			i = k
		}
	}
}

func isXMLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
