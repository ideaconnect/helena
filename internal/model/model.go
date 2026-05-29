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
	GET     Method = "GET"
	POST    Method = "POST"
	PUT     Method = "PUT"
	PATCH   Method = "PATCH"
	DELETE  Method = "DELETE"
	HEAD    Method = "HEAD"
	OPTIONS Method = "OPTIONS"
)

// Methods lists the HTTP methods Helena supports, in display order.
var Methods = []Method{GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS}

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
)

// BodyTypes lists supported body types in display order.
var BodyTypes = []BodyType{BodyNone, BodyJSON, BodyXML, BodyText, BodyForm, BodyMultipart}

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
	case BodyJSON:
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
}

// Request is a single HTTP request definition.
type Request struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Method  Method     `json:"method"`
	URL     string     `json:"url"`
	Params  []KeyValue `json:"params,omitempty"`
	Headers []KeyValue `json:"headers,omitempty"`
	Body    Body       `json:"body"`
	Docs    string     `json:"docs,omitempty"`    // free-form markdown
	Auth    Auth       `json:"auth,omitempty"`    // own auth or Inherit from parent
	Scripts Scripts    `json:"scripts,omitempty"` // pre/post JS hooks
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
	Auth         Auth          `json:"auth,omitempty"`
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

// Settings holds app-wide preferences.
type Settings struct {
	InsecureSkipVerify bool  `json:"insecureSkipVerify"` // allow invalid/self-signed TLS
	CORSWarning        bool  `json:"corsWarning"`        // show CORS advisory on responses
	FollowRedirects    bool  `json:"followRedirects"`
	TimeoutSeconds     int   `json:"timeoutSeconds"`
	Theme              Theme `json:"theme"`
}

// DefaultSettings returns Helena's default preferences.
func DefaultSettings() Settings {
	return Settings{
		InsecureSkipVerify: false,
		CORSWarning:        true,
		FollowRedirects:    true,
		TimeoutSeconds:     30,
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
