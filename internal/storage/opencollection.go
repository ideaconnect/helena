package storage

import "github.com/idct/helena/internal/model"

// OpenCollection YAML DTOs mirror the on-disk schema documented at
// https://docs.usebruno.com/opencollection-yaml. They are kept separate from
// the domain model so the two can evolve independently.
//
// Not yet mapped (round-tripped): auth, runtime scripts/assertions, per-request
// settings, docs, graphql/multipart body payloads, and the query/path param
// distinction. These are dropped on save for now.

type ocInfo struct {
	Name string   `yaml:"name"`
	Type string   `yaml:"type,omitempty"` // http | folder | collection | environment
	Seq  int      `yaml:"seq,omitempty"`
	Tags []string `yaml:"tags,omitempty"`
}

type ocKV struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled,omitempty"`
}

type ocParam struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Type     string `yaml:"type,omitempty"` // query | path
	Disabled bool   `yaml:"disabled,omitempty"`
}

type ocBody struct {
	Type string `yaml:"type"`
	Data string `yaml:"data,omitempty"`
}

type ocHTTP struct {
	Method  string    `yaml:"method"`
	URL     string    `yaml:"url"`
	Headers []ocKV    `yaml:"headers,omitempty"`
	Params  []ocParam `yaml:"params,omitempty"`
	Body    *ocBody   `yaml:"body,omitempty"`
}

type ocRequestFile struct {
	Info ocInfo  `yaml:"info"`
	HTTP *ocHTTP `yaml:"http,omitempty"`
}

type ocFolderFile struct {
	Info ocInfo `yaml:"info"`
}

type ocCollectionFile struct {
	Info ocInfo `yaml:"info"`
}

type ocEnvVar struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled,omitempty"`
	Secret   bool   `yaml:"secret,omitempty"`
}

// ocEnvironmentFile is a best-effort mapping: the environment-file schema is not
// fully pinned down in the public docs, so verify against the spec before
// relying on cross-tool interop.
type ocEnvironmentFile struct {
	Info ocInfo     `yaml:"info"`
	Vars []ocEnvVar `yaml:"vars,omitempty"`
}

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
	return ocRequestFile{
		Info: ocInfo{Name: r.Name, Type: "http", Seq: seq},
		HTTP: h,
	}
}

func fileToRequest(f ocRequestFile) model.Request {
	r := model.Request{ID: model.NewID(), Name: f.Info.Name, Body: model.Body{Type: model.BodyNone}}
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
	return r
}

func envToFile(e model.Environment, seq int) ocEnvironmentFile {
	f := ocEnvironmentFile{Info: ocInfo{Name: e.Name, Type: "environment", Seq: seq}}
	for _, v := range e.Variables {
		f.Vars = append(f.Vars, ocEnvVar{Name: v.Key, Value: v.Value, Disabled: !v.Enabled, Secret: v.Secret})
	}
	return f
}

func fileToEnv(f ocEnvironmentFile) model.Environment {
	e := model.Environment{ID: model.NewID(), Name: f.Info.Name}
	for _, v := range f.Vars {
		e.Variables = append(e.Variables, model.Variable{Enabled: !v.Disabled, Key: v.Name, Value: v.Value, Secret: v.Secret})
	}
	return e
}
