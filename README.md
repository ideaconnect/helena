# Helena 🐱

A super-lightweight, cross-platform API client — a native alternative to Postman
and Bruno — built with **Go + [Fyne](https://fyne.io)**. One self-contained
binary, no Electron.

> **Status: Phase 1 — scaffold.** Full build plan lives in
> [Asana](https://app.asana.com/1/1214897106264347/project/1215180905395792).

## Features (planned)

- Workspaces, collections & folders, stored as **Open Collection YAML**
- Per-collection environments with `{{variable}}` resolution everywhere
  (URL, query params, headers, body)
- Request builder: method + URL, query params, headers, body with JSON/XML validation
- Response viewer: raw / pretty JSON / pretty XML / headers
- Import OpenAPI 3, Swagger 2 and WSDL
- Export to cURL / WGET (more languages later)
- Settings: invalid-SSL toggle, CORS advisory, light/dark theme

## Requirements

- Go 1.23+ (developed on 1.26.3)
- A C compiler (Fyne uses cgo)
- **Linux GUI build dependencies** (Debian/Ubuntu):

  ```sh
  sudo apt-get install -y libgl1-mesa-dev xorg-dev
  ```

## Develop

```sh
make tidy    # resolve modules
make run     # run the app
make build   # build ./bin/helena
make test    # run tests
make lint    # golangci-lint
```

## Layout

| Path | Responsibility |
| --- | --- |
| `cmd/helena` | entrypoint |
| `internal/model` | domain types |
| `internal/storage` | Open Collection YAML load/save |
| `internal/vars` | `{{var}}` resolution |
| `internal/httpclient` | request execution |
| `internal/importer` | OpenAPI / Swagger / WSDL import |
| `internal/exporter` | cURL / WGET / … export |
| `internal/config` | settings + UI state |
| `internal/ui` | Fyne views |
| `assets` | embedded app icon |

## License

TBD.
