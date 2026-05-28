# Helena 🐱

A super-lightweight, cross-platform API client — a native alternative to
Postman and Bruno — built with **Go + [Fyne](https://fyne.io)**. One
self-contained binary, no Electron.

[![CI](https://github.com/idct/helena/actions/workflows/ci.yml/badge.svg)](https://github.com/idct/helena/actions/workflows/ci.yml)

## Features

- **Workspaces, collections, folders, requests** stored as plain Open
  Collection YAML on disk — version-control them like any other source code.
- **Environments per collection** with `{{variable}}` resolution everywhere
  (URL, query params, headers, body) plus a live preview of the resolved URL.
- **Request builder** — method, URL, query params, headers, body
  (JSON / XML / text / form-urlencoded / multipart). Validate + Format
  buttons for JSON and XML.
- **Response viewer** — raw, pretty JSON, pretty XML, headers. Status line
  shows `200 OK · 1.2 KB · 87 ms`.
- **CORS advisory.** Helena always sends the request (it isn't a browser),
  but flags responses a browser would have blocked.
- **Import** OpenAPI 3, Swagger 2, or WSDL — from a local file or a URL.
- **Export** to cURL or wget, with Copy-to-clipboard.
- **Settings** — invalid-SSL toggle, CORS warning, follow-redirects, request
  timeout, light/dark/system theme. Persisted in your OS's standard config
  dir (`AppData\Roaming` on Windows, `~/Library/Application Support` on
  macOS, `~/.config` on Linux).
- **Keyboard shortcuts** (Mod = Ctrl on Linux/Windows, ⌘ on macOS):
  - **Mod+Enter** Send · **Mod+S** Save · **Mod+O** Open · **Mod+I** Import
  - **Mod+N** New request · **Mod+Shift+N** New collection · **Mod+D** Duplicate
  - **Mod+E** Environments · **Mod+,** Settings · **F1** show all shortcuts

## Download

Pre-built Linux (amd64) and Windows (amd64) binaries are attached to each
[GitHub Release](https://github.com/idct/helena/releases). macOS builds
come later.

## Quickstart

To get a feel for Helena without writing a request from scratch, open the
bundled sample collection:

1. Launch Helena.
2. Click **Open…** in the Collections sidebar.
3. Select [examples/httpbin/](examples/httpbin/) inside this repo.

The sample has two requests hitting [httpbin.org](https://httpbin.org) — a
`GET /anything` with a query param and a `POST /anything` with a JSON body —
plus a `default` environment that provides `{{base_url}}`. Select either
request in the tree, press **Mod+Enter**, and you should see a structured
response.

## Build from source

Requirements:

- Go 1.23+ (project tracks Go via `go.mod`; currently `go 1.26.3`)
- A C compiler (Fyne uses cgo + OpenGL)
- **Linux**: `sudo apt-get install -y libgl1-mesa-dev xorg-dev`
- **Windows**: TDM-GCC or MSYS2 mingw-w64 on `PATH`
- **macOS**: Xcode Command Line Tools (`xcode-select --install`)

**Linux / macOS / WSL**

```sh
make tidy    # resolve modules
make run     # run the app
make build   # build ./bin/helena
make test    # run all tests
make lint    # golangci-lint (optional)
```

**Windows** (Cmd or PowerShell)

```cmd
make.bat tidy
make.bat run
make.bat build   :: writes bin\helena.exe
make.bat test
```

`Makefile` and `make.bat` expose the same targets with the same defaults.

## Layout

| Path | Responsibility |
| --- | --- |
| `cmd/helena` | application entrypoint |
| `internal/model` | domain types |
| `internal/storage` | Open Collection YAML load/save |
| `internal/vars` | `{{var}}` resolver |
| `internal/httpclient` | request execution + CORS advisory |
| `internal/responsefmt` | pretty-printing + content-type sniffing |
| `internal/importer` | OpenAPI / Swagger / WSDL (file or URL) |
| `internal/exporter` | cURL / wget export |
| `internal/config` | settings + UI state persistence |
| `internal/session` | runtime workspace + collection state |
| `internal/ui` | Fyne views |
| `assets` | embedded app icon |
| `.github/workflows` | CI: native Linux + Windows build matrix |

## Architecture notes

- **Storage round-trips unknown fields.** Every OpenCollection DTO embeds an
  `Extra map[string]yaml.Node` catch-all, so YAML keys written by other
  tools (auth, runtime scripts, custom docs, …) survive a load → save cycle
  even though Helena itself doesn't expose them in the UI yet.
- **CORS is advisory, not a toggle.** A native client can't actually enforce
  CORS. Helena compares the request `Origin` against the response
  `Access-Control-Allow-Origin` and shows an orange warning if a browser
  would have blocked the response. The request is sent regardless.
- **Native CI, no cross-compile.** GitHub Actions runs `ubuntu-latest` and
  `windows-latest` in a matrix so each binary is produced by its own OS's
  native cgo toolchain. No fyne-cross, no Docker. macOS deferred.

## License

BSD 4-Clause — see [LICENSE](LICENSE).
