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
	Extra   map[string]yaml.Node `yaml:",inline"` // catches auth and other http-level fields
}

type ocRequestFile struct {
	Info  ocInfo               `yaml:"info"`
	HTTP  *ocHTTP              `yaml:"http,omitempty"`
	Extra map[string]yaml.Node `yaml:",inline"` // catches runtime, settings, docs, …
}

type ocFolderFile struct {
	Info  ocInfo               `yaml:"info"`
	Extra map[string]yaml.Node `yaml:",inline"`
}

type ocCollectionFile struct {
	Info  ocInfo               `yaml:"info"`
	Extra map[string]yaml.Node `yaml:",inline"`
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
