package model

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Method is an HTTP request method.
type Method string

// Supported HTTP methods.
const (
	GET    Method = "GET"
	POST   Method = "POST"
	PUT    Method = "PUT"
	PATCH  Method = "PATCH"
	DELETE Method = "DELETE"
	// QUERY is the safe, idempotent, cacheable "GET with a body" standardized
	// by RFC 10008 (Proposed Standard, June 2026) and registered in IANA's HTTP
	// Method Registry. OpenAPI 3.2 gives it a dedicated path-item field, so
	// imported specs can carry QUERY operations.
	QUERY   Method = "QUERY"
	HEAD    Method = "HEAD"
	OPTIONS Method = "OPTIONS"
	TRACE   Method = "TRACE"
	CONNECT Method = "CONNECT"
)

// Methods lists the HTTP methods Helena supports, in display order.
var Methods = []Method{GET, POST, PUT, PATCH, DELETE, QUERY, HEAD, OPTIONS, TRACE, CONNECT}

// Valid reports whether m is a recognized HTTP method.
func (m Method) Valid() bool {
	for _, k := range Methods {
		if m == k {
			return true
		}
	}
	return false
}

// BodyType identifies how a request body is encoded.
type BodyType string

// Supported body types.
const (
	BodyNone      BodyType = "none"
	BodyJSON      BodyType = "json"
	BodyXML       BodyType = "xml"
	BodyText      BodyType = "text"
	BodyForm      BodyType = "form-urlencoded"
	BodyMultipart BodyType = "multipart-form"
	BodyFile      BodyType = "file"    // raw bytes of a file on disk (#24)
	BodyGraphQL   BodyType = "graphql" // query + variables, sent as a JSON envelope (#70)
)

// BodyTypes lists supported body types in display order.
var BodyTypes = []BodyType{BodyNone, BodyJSON, BodyXML, BodyText, BodyGraphQL, BodyForm, BodyMultipart, BodyFile}

// Valid reports whether t is a recognized body type.
func (t BodyType) Valid() bool {
	for _, k := range BodyTypes {
		if t == k {
			return true
		}
	}
	return false
}

// ContentType returns the Content-Type header implied by the body type, or ""
// when none applies: BodyNone has no body, and multipart needs a generated
// boundary that is set when the request is sent.
func (t BodyType) ContentType() string {
	switch t {
	case BodyJSON, BodyGraphQL:
		return "application/json"
	case BodyXML:
		return "application/xml"
	case BodyText:
		return "text/plain"
	case BodyForm:
		return "application/x-www-form-urlencoded"
	default:
		return ""
	}
}

// KeyValue is an enableable key/value pair, used for headers and query params.
type KeyValue struct {
	Enabled     bool   `json:"enabled"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// Body is a request body.
type Body struct {
	Type    BodyType   `json:"type"`
	Content string     `json:"content,omitempty"` // raw text for json/xml/text
	Form    []KeyValue `json:"form,omitempty"`    // fields for form-urlencoded/multipart
	// FilePath / ContentType back BodyFile (#24): the request sends the exact
	// bytes of the file at FilePath, advertised as ContentType (defaulting to
	// application/octet-stream). Both are empty for the other body types.
	FilePath    string `json:"filePath,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	// GraphQLVariables backs BodyGraphQL (#70): the raw JSON object of GraphQL
	// variables. Body.Content holds the query; at send time the two are combined
	// into a {"query":…,"variables":…} JSON envelope. Empty for other body types
	// (and the variables key is omitted when this is blank).
	GraphQLVariables string `json:"graphqlVariables,omitempty"`
}

// Request is a single HTTP request definition.
type Request struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Method Method     `json:"method"`
	URL    string     `json:"url"`
	Params []KeyValue `json:"params,omitempty"`
	// PathParams fill the single-brace {name} placeholders in URL's path (e.g.
	// {{base_url}}/bag/{bagId}), keyed by the placeholder name. They are a
	// separate scope from query Params: substituted into the path at send time
	// rather than appended to the query string, and stored as OpenCollection
	// `type: path` params. See internal/pathparam.
	PathParams []KeyValue  `json:"pathParams,omitempty"`
	Headers    []KeyValue  `json:"headers,omitempty"`
	Body       Body        `json:"body"`
	Docs       string      `json:"docs,omitempty"`    // free-form markdown
	Auth       Auth        `json:"auth,omitempty"`    // own auth or Inherit from parent
	Scripts    Scripts     `json:"scripts,omitempty"` // pre/post JS hooks
	Chain      []ChainStep `json:"chain,omitempty"`   // before-hooks (linear list)
	// Assertions are declarative (no-code) response checks (#88), evaluated
	// after Send and reported alongside test()/expect() results.
	Assertions []Assertion `json:"assertions,omitempty"`
	// Variables are request-scoped variables — the highest-precedence static
	// scope (above environment and collection; only the runtime script overlay
	// wins). They apply only when this request is the one being sent.
	Variables []Variable `json:"variables,omitempty"`
}

// Clone returns a deep copy of r with every reference-typed field detached from
// the original's backing arrays: the slice fields (Params, PathParams, Headers,
// Body.Form, Chain, Assertions, Variables) and Auth's scheme sub-structs.
// KeyValue / ChainStep / Assertion / Variable are flat value structs, so a slice
// copy fully detaches them. This is the single home for Request deep-copying —
// the off-UI send/stream snapshot, the history record, and the secret scrubber
// all route through it so no caller can mutate another's request and no field
// list drifts.
func (r Request) Clone() Request {
	r.Params = append([]KeyValue(nil), r.Params...)
	r.PathParams = append([]KeyValue(nil), r.PathParams...)
	r.Headers = append([]KeyValue(nil), r.Headers...)
	r.Body.Form = append([]KeyValue(nil), r.Body.Form...)
	r.Chain = append([]ChainStep(nil), r.Chain...)
	r.Assertions = append([]Assertion(nil), r.Assertions...)
	r.Variables = append([]Variable(nil), r.Variables...)
	r.Auth = r.Auth.Clone()
	return r
}

// ChainStep names another request to execute before this one, binding
// the result to an alias so this request's scripts can read it via the
// `chain` global (e.g. chain.login.response.json.token). Request is a
// slash-separated name-path within the same collection: `"Auth/Login"`
// resolves to the request named "Login" inside the folder named
// "Auth". Names are case-sensitive and match the on-disk display name.
//
// RequestID, when non-empty, is the persistent Request.ID of the
// target — the chain runner prefers it over Request for resolution so
// the ref survives renames and folder moves without depending on the
// path. Request is still written for human readability (and as a
// fallback when the ID can't be resolved, e.g. across collections or
// in YAML hand-edited by other tools).
type ChainStep struct {
	Alias     string `json:"alias"`
	Request   string `json:"request"`
	RequestID string `json:"requestId,omitempty"`
}

// Assertion is one declarative response check (#88). Source is an expression
// over the response (e.g. "res.status", "res.body", "res.header.Content-Type",
// "res.json.data.0.id"); Op is one of the evaluator's operators; Expected is
// the comparison value (ignored for exists/notExists).
type Assertion struct {
	Enabled  bool   `json:"enabled"`
	Source   string `json:"source"`
	Op       string `json:"op"`
	Expected string `json:"expected,omitempty"`
}

// Scripts holds the per-request JavaScript hooks the scripting runtime
// executes around Send. Both fields are raw ECMAScript source — empty
// strings disable that hook. The runtime is in-memory and dies with the
// process; scripts have no persistent side effects beyond Helena's
// in-memory environment overlay.
type Scripts struct {
	PreRequest   string `json:"preRequest,omitempty"`
	PostResponse string `json:"postResponse,omitempty"`
}

// IsEmpty reports whether neither hook has any non-whitespace content,
// which the UI and Send pipeline use to skip the scripting runtime
// entirely.
func (s Scripts) IsEmpty() bool {
	return strings.TrimSpace(s.PreRequest) == "" && strings.TrimSpace(s.PostResponse) == ""
}

// Folder groups requests and nested folders within a collection. Auth
// applies to every descendant whose own Auth is Inherit.
type Folder struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Folders  []Folder  `json:"folders,omitempty"`
	Requests []Request `json:"requests,omitempty"`
	Auth     Auth      `json:"auth,omitempty"`
	// Variables are folder-scoped variables (#81): a resolver scope between the
	// environment and the request — nearer (inner) folders override outer ones.
	Variables []Variable `json:"variables,omitempty"`
}

// Variable is a single environment variable.
type Variable struct {
	Enabled bool   `json:"enabled"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Secret  bool   `json:"secret,omitempty"`
}

// Environment is a named set of variables (e.g. Local, Staging, Prod).
type Environment struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Variables []Variable `json:"variables,omitempty"`
}

// Collection is a tree of folders and requests plus its own environments.
// Auth on the collection root is the outermost ancestor in the
// auth-inheritance walk; folders and requests with Auth=Inherit fall back
// to it.
type Collection struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Folders      []Folder      `json:"folders,omitempty"`
	Requests     []Request     `json:"requests,omitempty"`
	Environments []Environment `json:"environments,omitempty"`
	// Variables are collection-scoped variables — a resolver scope BELOW the
	// environment (an environment value of the same name wins). Unlike
	// Environments they are not selectable; they always apply to the collection.
	Variables []Variable `json:"variables,omitempty"`
	Auth      Auth       `json:"auth,omitempty"`
}

// Workspace groups collections under a single roof.
type Workspace struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Collections []Collection `json:"collections,omitempty"`
}

// Theme selects the UI appearance.
type Theme string

// Available themes.
const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// DefaultMaxResponseBytes is the built-in response-body buffer cap (100 MiB),
// used when Settings.MaxResponseBytes is unset (<=0). Helena is a desktop
// client, not a stream processor, so an unbounded read could OOM the app.
const DefaultMaxResponseBytes int64 = 100 << 20

// Settings holds app-wide preferences.
type Settings struct {
	InsecureSkipVerify bool  `json:"insecureSkipVerify"` // allow invalid/self-signed TLS
	CORSWarning        bool  `json:"corsWarning"`        // show CORS advisory on responses
	FollowRedirects    bool  `json:"followRedirects"`
	TimeoutSeconds     int   `json:"timeoutSeconds"`
	MaxResponseBytes   int64 `json:"maxResponseBytes"` // buffered-body cap; <=0 uses DefaultMaxResponseBytes
	Theme              Theme `json:"theme"`
}

// DefaultSettings returns Helena's default preferences.
func DefaultSettings() Settings {
	return Settings{
		InsecureSkipVerify: false,
		CORSWarning:        true,
		FollowRedirects:    true,
		TimeoutSeconds:     30,
		MaxResponseBytes:   DefaultMaxResponseBytes,
		Theme:              ThemeSystem,
	}
}

// EnabledPairs returns only the enabled key/value pairs, preserving order.
func EnabledPairs(kvs []KeyValue) []KeyValue {
	out := make([]KeyValue, 0, len(kvs))
	for _, kv := range kvs {
		if kv.Enabled {
			out = append(out, kv)
		}
	}
	return out
}

// NewID returns a random 128-bit hex identifier for tree items.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
