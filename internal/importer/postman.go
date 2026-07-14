package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idct/helena/internal/model"
)

// FromPostman parses a Postman Collection v2.1 (or v2.0) JSON document and
// returns the resulting Helena collection. Folders, requests, headers, query
// params, request bodies (raw / urlencoded / formdata / graphql), and the
// common auth schemes (bearer / basic / apikey / noauth) are mapped; Postman
// features Helena has no home for (events/scripts, response examples, file
// bodies) are dropped rather than failing the import.
func FromPostman(data []byte) (model.Collection, error) {
	var pc pmCollection
	if err := json.Unmarshal(data, &pc); err != nil {
		return model.Collection{}, fmt.Errorf("postman parse: %w", err)
	}
	c := model.Collection{
		ID:   model.NewID(),
		Name: stringOr(pc.Info.Name, "Imported Collection"),
		Auth: pmConvertAuth(pc.Auth),
	}
	for _, it := range pc.Item {
		c.Folders, c.Requests = pmAppendItem(it, c.Folders, c.Requests)
	}
	return c, nil
}

// looksLikePostman reports whether data is a Postman collection — it has both
// an `info` object and an `item` array and is not an OpenAPI/Swagger document
// (those carry an `openapi` or `swagger` key and would otherwise also have
// neither `info`+`item` pair).
func looksLikePostman(data []byte) bool {
	var probe struct {
		Info    *json.RawMessage `json:"info"`
		Item    *json.RawMessage `json:"item"`
		OpenAPI *json.RawMessage `json:"openapi"`
		Swagger *json.RawMessage `json:"swagger"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	if probe.OpenAPI != nil || probe.Swagger != nil {
		return false
	}
	return probe.Info != nil && probe.Item != nil
}

// pmAppendItem converts one Postman item — a folder (has nested items) or a
// request — and appends it to the running folder/request slices, returning the
// grown slices. An item with a non-nil request is a leaf; otherwise it is a
// folder (possibly empty).
func pmAppendItem(it pmItem, folders []model.Folder, requests []model.Request) ([]model.Folder, []model.Request) {
	if it.Request != nil {
		return folders, append(requests, pmConvertRequest(it))
	}
	f := model.Folder{ID: model.NewID(), Name: it.Name, Auth: pmConvertAuth(it.Auth)}
	for _, child := range it.Item {
		f.Folders, f.Requests = pmAppendItem(child, f.Folders, f.Requests)
	}
	return append(folders, f), requests
}

// pmConvertRequest maps a Postman request item to a model.Request.
func pmConvertRequest(it pmItem) model.Request {
	rq := it.Request
	r := model.Request{
		ID:     model.NewID(),
		Name:   stringOr(it.Name, rq.URL.Raw),
		Method: pmMethod(rq.Method),
		URL:    rq.URL.effectiveRaw(),
		Docs:   rq.Description,
		Body:   model.Body{Type: model.BodyNone},
		Auth:   pmConvertAuth(rq.Auth),
	}
	for _, h := range rq.Header {
		r.Headers = append(r.Headers, model.KeyValue{
			Enabled:     !h.Disabled,
			Key:         h.Key,
			Value:       h.Value,
			Description: h.Description,
		})
	}
	for _, q := range rq.URL.Query {
		r.Params = append(r.Params, model.KeyValue{
			Enabled:     !q.Disabled,
			Key:         q.Key,
			Value:       q.Value,
			Description: q.Description,
		})
	}
	if rq.Body != nil {
		r.Body = pmConvertBody(rq.Body)
	}
	return r
}

// pmMethod normalizes a Postman method string to a model.Method, defaulting to
// GET when empty.
func pmMethod(m string) model.Method {
	if m == "" {
		return model.GET
	}
	return model.Method(strings.ToUpper(m))
}

// pmConvertBody maps a Postman body block to a model.Body by mode. GraphQL is
// stored as a JSON body carrying the query + variables since Helena has no
// dedicated GraphQL body type; file bodies have no embeddable content and map
// to no body.
func pmConvertBody(b *pmBody) model.Body {
	switch b.Mode {
	case "raw":
		return model.Body{Type: pmRawBodyType(b), Content: b.Raw}
	case "urlencoded":
		return model.Body{Type: model.BodyForm, Form: pmFormParams(b.URLEncoded)}
	case "formdata":
		return model.Body{Type: model.BodyMultipart, Form: pmFormParams(b.FormData)}
	case "graphql":
		if b.GraphQL == nil {
			return model.Body{Type: model.BodyNone}
		}
		payload := map[string]any{"query": b.GraphQL.Query}
		if v := strings.TrimSpace(b.GraphQL.Variables); v != "" {
			payload["variables"] = json.RawMessage(v)
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return model.Body{Type: model.BodyText, Content: b.GraphQL.Query}
		}
		return model.Body{Type: model.BodyJSON, Content: string(out)}
	default:
		return model.Body{Type: model.BodyNone}
	}
}

// pmRawBodyType picks a body type for a raw body from its declared language
// (options.raw.language), defaulting to text.
func pmRawBodyType(b *pmBody) model.BodyType {
	lang := strings.ToLower(b.Options.Raw.Language)
	switch {
	case strings.Contains(lang, "json"):
		return model.BodyJSON
	case strings.Contains(lang, "xml"):
		return model.BodyXML
	default:
		return model.BodyText
	}
}

// pmFormParams maps Postman urlencoded/formdata params to model KeyValues.
func pmFormParams(params []pmFormParam) []model.KeyValue {
	var out []model.KeyValue
	for _, p := range params {
		out = append(out, model.KeyValue{
			Enabled:     !p.Disabled,
			Key:         p.Key,
			Value:       p.Value,
			Description: p.Description,
		})
	}
	return out
}

// pmConvertAuth maps a Postman auth block to a model.Auth. Unknown or absent
// auth yields the zero Auth (treated as Inherit on load); "noauth" maps to an
// explicit None.
func pmConvertAuth(a *pmAuth) model.Auth {
	if a == nil {
		return model.Auth{}
	}
	switch a.Type {
	case "noauth":
		return model.Auth{Type: model.AuthNone}
	case "bearer":
		return model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: pmAuthValue(a.Bearer, "token")}}
	case "basic":
		return model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{
			Username: pmAuthValue(a.Basic, "username"),
			Password: pmAuthValue(a.Basic, "password"),
		}}
	case "apikey":
		placement := model.APIKeyHeader
		if strings.EqualFold(pmAuthValue(a.APIKey, "in"), "query") {
			placement = model.APIKeyQuery
		}
		return model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{
			Name:      pmAuthValue(a.APIKey, "key"),
			Value:     pmAuthValue(a.APIKey, "value"),
			Placement: placement,
		}}
	default:
		return model.Auth{}
	}
}

// pmAuthValue pulls the value of a named key from a Postman auth parameter
// list (e.g. the "token" entry of a bearer block), stringifying non-string
// values. Returns "" when the key is absent.
func pmAuthValue(params []pmAuthParam, key string) string {
	for _, p := range params {
		if p.Key == key {
			if s, ok := p.Value.(string); ok {
				return s
			}
			if p.Value == nil {
				return ""
			}
			return fmt.Sprint(p.Value)
		}
	}
	return ""
}

// Postman Collection v2.x decode structs. Only the fields Helena maps are
// declared; everything else in the document is ignored by encoding/json.

type pmCollection struct {
	Info pmInfo   `json:"info"`
	Item []pmItem `json:"item"`
	Auth *pmAuth  `json:"auth"`
}

type pmInfo struct {
	Name string `json:"name"`
}

type pmItem struct {
	Name    string     `json:"name"`
	Item    []pmItem   `json:"item"`
	Request *pmRequest `json:"request"`
	Auth    *pmAuth    `json:"auth"`
}

type pmRequest struct {
	Method      string     `json:"method"`
	Header      []pmHeader `json:"header"`
	URL         pmURL      `json:"url"`
	Body        *pmBody    `json:"body"`
	Auth        *pmAuth    `json:"auth"`
	Description string     `json:"description"`
}

// UnmarshalJSON accepts Postman's two `request` forms: a bare URL string
// shorthand (`"request": "https://x"`), which the v2.1 schema defines as a GET
// to that URL, or the full request object. Without this a collection that uses
// the string shorthand for even one request fails the entire import.
func (r *pmRequest) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*r = pmRequest{URL: pmURL{Raw: s}} // method left empty → pmMethod defaults to GET
		return nil
	}
	// Alias sheds this method so the object decode doesn't recurse; the nested
	// pmURL/pmBody/... still use their own UnmarshalJSON.
	type raw pmRequest
	var rr raw
	if err := json.Unmarshal(b, &rr); err != nil {
		return err
	}
	*r = pmRequest(rr)
	return nil
}

type pmHeader struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Description string `json:"description"`
}

// pmURL is either a bare string ("https://x/y?a=1") or an object with a raw
// form plus structured host/path/query parts. UnmarshalJSON accepts both.
type pmURL struct {
	Raw   string
	Host  []string
	Path  []string
	Query []pmQuery
}

func (u *pmURL) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		u.Raw = s
		return nil
	}
	var obj struct {
		Raw   string          `json:"raw"`
		Host  json.RawMessage `json:"host"`
		Path  json.RawMessage `json:"path"`
		Query []pmQuery       `json:"query"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	u.Raw = obj.Raw
	u.Host = pmStringList(obj.Host)
	u.Path = pmStringList(obj.Path)
	u.Query = obj.Query
	return nil
}

// effectiveRaw returns the request URL — the raw string when present, else a
// best-effort reconstruction from the host and path parts (Postman almost
// always supplies raw, but exporters occasionally omit it).
func (u pmURL) effectiveRaw() string {
	if u.Raw != "" {
		return u.Raw
	}
	host := strings.Join(u.Host, ".")
	path := strings.Join(u.Path, "/")
	if host == "" {
		return path
	}
	if path == "" {
		return host
	}
	return host + "/" + path
}

// pmStringList decodes a Postman host/path value, which may be a JSON array of
// strings or a single string, into a string slice.
func pmStringList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return []string{s}
	}
	return nil
}

type pmQuery struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Description string `json:"description"`
}

type pmBody struct {
	Mode       string        `json:"mode"`
	Raw        string        `json:"raw"`
	Options    pmBodyOptions `json:"options"`
	URLEncoded []pmFormParam `json:"urlencoded"`
	FormData   []pmFormParam `json:"formdata"`
	GraphQL    *pmGraphQL    `json:"graphql"`
}

type pmBodyOptions struct {
	Raw struct {
		Language string `json:"language"`
	} `json:"raw"`
}

type pmFormParam struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Description string `json:"description"`
}

type pmGraphQL struct {
	Query     string `json:"query"`
	Variables string `json:"variables"`
}

type pmAuth struct {
	Type   string        `json:"type"`
	Bearer []pmAuthParam `json:"bearer"`
	Basic  []pmAuthParam `json:"basic"`
	APIKey []pmAuthParam `json:"apikey"`
}

type pmAuthParam struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}
