# storage

Package `storage` loads and saves Helena collections in the
[OpenCollection YAML](https://docs.usebruno.com/opencollection-yaml) format and
maps that on-disk layout to and from the domain types in
[`internal/model`](../model/).

A collection on disk is a directory: an `opencollection.yml` at the root,
one `.yml` file per request, one subdirectory (with its own `folder.yml`) per
folder, and an `environments/` directory holding one `.yml` per environment.
Helena models only a subset of the schema — the rest (auth blocks, runtime
scripts, per-request settings, custom fields on headers/params, …) is
round-tripped through `Extra map[string]yaml.Node` catch-alls on every DTO so
externally-authored collections survive Helena's load → save cycle without
losing data. See [WORKFLOW.md](WORKFLOW.md) for the exact flow.

The package is intentionally split into a DTO layer that mirrors the YAML
schema ([opencollection.go](opencollection.go)) and a directory walker that
performs IO and the model conversion ([store.go](store.go)).

## Public API

- `Save(c model.Collection, dir string) error` — write a collection to `dir`
  in OpenCollection layout, preserving unknown fields from existing files and
  sweeping orphans afterwards.
- `Load(dir string) (model.Collection, error)` — read a collection from `dir`,
  assigning fresh IDs (the format does not record them).

## Dependencies

- [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML marshalling
  with support for `,inline` catch-all maps via `yaml.Node`.
- [`internal/model`](../model/) — domain types (`Collection`, `Folder`,
  `Request`, `Environment`, `Variable`, `KeyValue`, `Body`).

Standard library only otherwise (`os`, `path/filepath`, `regexp`, `sort`,
`strings`).
