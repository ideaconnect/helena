package storage

import (
	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// OpenCollection YAML DTOs mirror the on-disk schema documented at
// https://docs.usebruno.com/opencollection-yaml. They are kept separate from
// the domain model so the two can evolve independently.
//
// Each DTO carries an `Extra map[string]yaml.Node `yaml:",inline"`` catch-all
// so unknown fields from externally-created collections (auth, runtime scripts,
// per-request settings, docs, custom keys on headers/params, …) round-trip
// through a load → save cycle without being lost.

type ocInfo struct {
	Name  string               `yaml:"name"`
	ID    string               `yaml:"id,omitempty"`   // Helena-stable identifier; survives renames + folder moves. Optional on disk for back-compat — see fileToRequest.
	Type  string               `yaml:"type,omitempty"` // http | folder | collection | environment
	Seq   int                  `yaml:"seq,omitempty"`
	Tags  []string             `yaml:"tags,omitempty"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocKV struct {
	Name     string               `yaml:"name"`
	Value    string               `yaml:"value"`
	Disabled bool                 `yaml:"disabled,omitempty"`
	Extra    map[string]yaml.Node `yaml:",inline"`
}

type ocParam struct {
	Name     string               `yaml:"name"`
	Value    string               `yaml:"value"`
	Type     string               `yaml:"type,omitempty"` // query | path
	Disabled bool                 `yaml:"disabled,omitempty"`
	Extra    map[string]yaml.Node `yaml:",inline"`
}

type ocBody struct {
	Type  string               `yaml:"type"`
	Data  string               `yaml:"data,omitempty"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocHTTP struct {
	Method  string               `yaml:"method"`
	URL     string               `yaml:"url"`
	Headers []ocKV               `yaml:"headers,omitempty"`
	Params  []ocParam            `yaml:"params,omitempty"`
	Body    *ocBody              `yaml:"body,omitempty"`
	Auth    *ocAuth              `yaml:"auth,omitempty"`
	Extra   map[string]yaml.Node `yaml:",inline"` // catches runtime + other http-level fields
}

type ocRequestFile struct {
	Info    ocInfo               `yaml:"info"`
	HTTP    *ocHTTP              `yaml:"http,omitempty"`
	Docs    string               `yaml:"docs,omitempty"` // free-form markdown
	Scripts *ocScripts           `yaml:"scripts,omitempty"`
	Chain   []ocChainStep        `yaml:"chain,omitempty"`
	Extra   map[string]yaml.Node `yaml:",inline"` // catches settings, runtime, …
}

// ocChainStep mirrors one entry under the on-disk `chain:` list. Extra
// preserves keys other tools may have nested on a chain entry.
// RequestID, when non-empty, pins the ref to the target's persistent
// Request.ID so the entry survives the target being renamed or moved
// to a different folder.
type ocChainStep struct {
	Alias     string               `yaml:"alias"`
	Request   string               `yaml:"request"`
	RequestID string               `yaml:"requestId,omitempty"`
	Extra     map[string]yaml.Node `yaml:",inline"`
}

// ocScripts mirrors the on-disk scripts block. preRequest and
// postResponse hold raw JavaScript source. Extra preserves keys other
// tools may have added (e.g. test runners).
type ocScripts struct {
	PreRequest   string               `yaml:"preRequest,omitempty"`
	PostResponse string               `yaml:"postResponse,omitempty"`
	Extra        map[string]yaml.Node `yaml:",inline"`
}

type ocFolderFile struct {
	Info  ocInfo               `yaml:"info"`
	Auth  *ocAuth              `yaml:"auth,omitempty"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocCollectionFile struct {
	Info  ocInfo               `yaml:"info"`
	Auth  *ocAuth              `yaml:"auth,omitempty"`
	Vars  []ocEnvVar           `yaml:"vars,omitempty"` // collection-scoped variables (#80)
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocAuth struct {
	Type   string               `yaml:"type"`
	Basic  *ocAuthBasic         `yaml:"basic,omitempty"`
	Bearer *ocAuthBearer        `yaml:"bearer,omitempty"`
	APIKey *ocAuthAPIKey        `yaml:"apikey,omitempty"`
	OAuth2 *ocAuthOAuth2        `yaml:"oauth2,omitempty"`
	Extra  map[string]yaml.Node `yaml:",inline"`
}

type ocAuthBasic struct {
	Username string               `yaml:"username"`
	Password string               `yaml:"password"`
	Extra    map[string]yaml.Node `yaml:",inline"`
}

type ocAuthBearer struct {
	Token string               `yaml:"token"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocAuthAPIKey struct {
	Name      string               `yaml:"name"`
	Value     string               `yaml:"value"`
	Placement string               `yaml:"placement,omitempty"`
	Extra     map[string]yaml.Node `yaml:",inline"`
}

type ocAuthOAuth2 struct {
	Grant        string               `yaml:"grant"`
	TokenURL     string               `yaml:"tokenUrl"`
	AuthURL      string               `yaml:"authUrl,omitempty"`
	ClientID     string               `yaml:"clientId"`
	ClientSecret string               `yaml:"clientSecret"`
	Scope        string               `yaml:"scope,omitempty"`
	RedirectURI  string               `yaml:"redirectUri,omitempty"`
	UsePKCE      bool                 `yaml:"usePkce,omitempty"`
	Audience     string               `yaml:"audience,omitempty"`
	Extra        map[string]yaml.Node `yaml:",inline"`
}

type ocEnvVar struct {
	Name     string               `yaml:"name"`
	Value    string               `yaml:"value"`
	Disabled bool                 `yaml:"disabled,omitempty"`
	Secret   bool                 `yaml:"secret,omitempty"`
	Extra    map[string]yaml.Node `yaml:",inline"`
}

// ocEnvironmentFile: the environment-file schema is not fully pinned down in
// the public docs yet, so this is a best-effort mapping; the Extra catch-all
// preserves whatever else is in the file.
type ocEnvironmentFile struct {
	Info  ocInfo               `yaml:"info"`
	Vars  []ocEnvVar           `yaml:"vars,omitempty"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

// requestToFile maps a model.Request to its on-disk DTO at sequence position
// seq. The Extra catch-alls are left nil — the Save path layers any
// previously-loaded extras on top before writing.
func requestToFile(r model.Request, seq int) ocRequestFile {
	method := r.Method
	if method == "" {
		method = model.GET
	}
	h := &ocHTTP{Method: string(method), URL: r.URL}
	for _, kv := range r.Headers {
		h.Headers = append(h.Headers, ocKV{Name: kv.Key, Value: kv.Value, Disabled: !kv.Enabled})
	}
	for _, p := range r.Params {
		h.Params = append(h.Params, ocParam{Name: p.Key, Value: p.Value, Type: "query", Disabled: !p.Enabled})
	}
	if r.Body.Type != "" && r.Body.Type != model.BodyNone {
		h.Body = &ocBody{Type: string(r.Body.Type), Data: r.Body.Content}
	}
	h.Auth = authToFile(r.Auth)
	return ocRequestFile{
		Info:    ocInfo{Name: r.Name, ID: r.ID, Type: "http", Seq: seq},
		HTTP:    h,
		Docs:    r.Docs,
		Scripts: scriptsToFile(r.Scripts),
		Chain:   chainToFile(r.Chain),
	}
}

// chainToFile mirrors a Chain slice into its on-disk DTO form. Returns
// nil for an empty slice so the YAML stays clean for non-chained
// requests.
func chainToFile(chain []model.ChainStep) []ocChainStep {
	if len(chain) == 0 {
		return nil
	}
	out := make([]ocChainStep, len(chain))
	for i, s := range chain {
		out[i] = ocChainStep{Alias: s.Alias, Request: s.Request, RequestID: s.RequestID}
	}
	return out
}

// fileToChain maps the on-disk chain DTO back into the domain model.
// Returns nil for an empty slice.
func fileToChain(chain []ocChainStep) []model.ChainStep {
	if len(chain) == 0 {
		return nil
	}
	out := make([]model.ChainStep, len(chain))
	for i, s := range chain {
		out[i] = model.ChainStep{Alias: s.Alias, Request: s.Request, RequestID: s.RequestID}
	}
	return out
}

// scriptsToFile returns nil when both hooks are empty so the on-disk
// scripts block stays out of the YAML when there's nothing to persist.
func scriptsToFile(s model.Scripts) *ocScripts {
	if s.PreRequest == "" && s.PostResponse == "" {
		return nil
	}
	return &ocScripts{PreRequest: s.PreRequest, PostResponse: s.PostResponse}
}

// fileToScripts maps the on-disk scripts DTO back into the domain
// model. A nil DTO produces the zero Scripts value (both hooks empty).
func fileToScripts(f *ocScripts) model.Scripts {
	if f == nil {
		return model.Scripts{}
	}
	return model.Scripts{PreRequest: f.PreRequest, PostResponse: f.PostResponse}
}

// fileToRequest maps an on-disk request DTO into the domain model. If
// the YAML carries `info.id` (written by an earlier Helena save) that
// ID is preserved so chain-step `requestId` refs stay valid across
// reloads. Files written by older Helena (or by Bruno) lack `info.id`
// — a fresh ID is generated; the next Save will write it back so the
// next reload preserves it. Missing auth resolves to AuthInherit so
// newly created requests default to inheriting their parent's auth.
func fileToRequest(f ocRequestFile) model.Request {
	id := f.Info.ID
	if id == "" {
		id = model.NewID()
	}
	r := model.Request{
		ID:      id,
		Name:    f.Info.Name,
		Body:    model.Body{Type: model.BodyNone},
		Docs:    f.Docs,
		Auth:    model.Auth{Type: model.AuthInherit},
		Scripts: fileToScripts(f.Scripts),
		Chain:   fileToChain(f.Chain),
	}
	if f.HTTP == nil {
		return r
	}
	r.Method = model.Method(f.HTTP.Method)
	r.URL = f.HTTP.URL
	for _, h := range f.HTTP.Headers {
		r.Headers = append(r.Headers, model.KeyValue{Enabled: !h.Disabled, Key: h.Name, Value: h.Value})
	}
	for _, p := range f.HTTP.Params {
		r.Params = append(r.Params, model.KeyValue{Enabled: !p.Disabled, Key: p.Name, Value: p.Value})
	}
	if f.HTTP.Body != nil {
		r.Body = model.Body{Type: model.BodyType(f.HTTP.Body.Type), Content: f.HTTP.Body.Data}
	}
	if f.HTTP.Auth != nil {
		r.Auth = fileToAuth(f.HTTP.Auth)
	}
	return r
}

// varsToFile / fileToVars map between a model.Variable slice and the on-disk
// ocEnvVar slice. Shared by environments and collection-level variables (#80).
func varsToFile(vars []model.Variable) []ocEnvVar {
	var out []ocEnvVar
	for _, v := range vars {
		out = append(out, ocEnvVar{Name: v.Key, Value: v.Value, Disabled: !v.Enabled, Secret: v.Secret})
	}
	return out
}

func fileToVars(ocs []ocEnvVar) []model.Variable {
	var out []model.Variable
	for _, v := range ocs {
		out = append(out, model.Variable{Enabled: !v.Disabled, Key: v.Name, Value: v.Value, Secret: v.Secret})
	}
	return out
}

// envToFile maps a model.Environment to its on-disk DTO at sequence position seq.
func envToFile(e model.Environment, seq int) ocEnvironmentFile {
	return ocEnvironmentFile{
		Info: ocInfo{Name: e.Name, Type: "environment", Seq: seq},
		Vars: varsToFile(e.Variables),
	}
}

// fileToEnv maps an on-disk environment DTO back into the domain model,
// assigning a fresh ID.
func fileToEnv(f ocEnvironmentFile) model.Environment {
	return model.Environment{ID: model.NewID(), Name: f.Info.Name, Variables: fileToVars(f.Vars)}
}

// authToFile maps a model.Auth to its on-disk DTO. Returns nil for the
// "" / Inherit case so the auth block stays out of the YAML when the
// caller has nothing meaningful to persist. Sub-structs that don't match
// the active Type are dropped.
func authToFile(a model.Auth) *ocAuth {
	if a.Type == "" || a.Type == model.AuthInherit {
		return nil
	}
	out := &ocAuth{Type: string(a.Type)}
	switch a.Type {
	case model.AuthBasic:
		if a.Basic != nil {
			out.Basic = &ocAuthBasic{Username: a.Basic.Username, Password: a.Basic.Password}
		}
	case model.AuthBearer:
		if a.Bearer != nil {
			out.Bearer = &ocAuthBearer{Token: a.Bearer.Token}
		}
	case model.AuthAPIKey:
		if a.APIKey != nil {
			out.APIKey = &ocAuthAPIKey{Name: a.APIKey.Name, Value: a.APIKey.Value, Placement: string(a.APIKey.Placement)}
		}
	case model.AuthOAuth2:
		if a.OAuth2 != nil {
			out.OAuth2 = &ocAuthOAuth2{
				Grant:        string(a.OAuth2.Grant),
				TokenURL:     a.OAuth2.TokenURL,
				AuthURL:      a.OAuth2.AuthURL,
				ClientID:     a.OAuth2.ClientID,
				ClientSecret: a.OAuth2.ClientSecret,
				Scope:        a.OAuth2.Scope,
				RedirectURI:  a.OAuth2.RedirectURI,
				UsePKCE:      a.OAuth2.UsePKCE,
				Audience:     a.OAuth2.Audience,
			}
		}
	}
	return out
}

// fileToAuth maps an on-disk auth DTO back into the domain model. A nil
// DTO is the caller's signal to fall back to AuthInherit (the default for
// requests and folders) — fileToAuth itself only handles non-nil input.
func fileToAuth(f *ocAuth) model.Auth {
	if f == nil {
		return model.Auth{Type: model.AuthInherit}
	}
	a := model.Auth{Type: model.AuthType(f.Type)}
	if f.Basic != nil {
		a.Basic = &model.BasicAuth{Username: f.Basic.Username, Password: f.Basic.Password}
	}
	if f.Bearer != nil {
		a.Bearer = &model.BearerAuth{Token: f.Bearer.Token}
	}
	if f.APIKey != nil {
		a.APIKey = &model.APIKeyAuth{
			Name:      f.APIKey.Name,
			Value:     f.APIKey.Value,
			Placement: model.APIKeyPlacement(f.APIKey.Placement),
		}
	}
	if f.OAuth2 != nil {
		a.OAuth2 = &model.OAuth2Auth{
			Grant:        model.OAuth2Grant(f.OAuth2.Grant),
			TokenURL:     f.OAuth2.TokenURL,
			AuthURL:      f.OAuth2.AuthURL,
			ClientID:     f.OAuth2.ClientID,
			ClientSecret: f.OAuth2.ClientSecret,
			Scope:        f.OAuth2.Scope,
			RedirectURI:  f.OAuth2.RedirectURI,
			UsePKCE:      f.OAuth2.UsePKCE,
			Audience:     f.OAuth2.Audience,
		}
	}
	return a
}
