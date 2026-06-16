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
- **Scripting & request chaining** — per-request pre/post JavaScript hooks, and
  before-hooks that run other requests first and feed their results into the
  next one. See [Request chaining](#request-chaining).
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
  timeout, max response size (MiB), light/dark/system theme. Persisted in your OS's standard config
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

## Request chaining

A request can declare **before-hooks** — other requests that Helena runs first,
in order, every time you Send it. Each hook gets an **alias**, and the
predecessor's result is then available to the dependent request. This is how you
feed a value produced by one request (a login token, the id of a record you just
created) into another.

**Declare the step.** On the dependent request, open the **Chain** tab and add a
row: an alias (say `login`) and the path to another request in the same
collection (say `Auth/Login`). On Send, `Auth/Login` executes first; then your
request runs with `chain.login` populated. Chaining is recursive (a hook's own
hooks run first) and cycle-checked; a request only ever sees its own aliases.

### Use a chained result as a `{{variable}}`

`{{chain.<alias>.…}}` resolves anywhere a normal `{{variable}}` does — URL, query
params, headers, **body, and auth fields**. To send a login token as the Bearer
credential, set the request's **Auth → Bearer Token** to:

    {{chain.login.response.json.token}}

The available paths:

| Template | Resolves to |
| --- | --- |
| `{{chain.<alias>.response.json.<path>}}` | a field of the parsed JSON body — dotted (`data.user.name`), array elements by index (`items.0.id`) |
| `{{chain.<alias>.response.headers.<Name>}}` | a response header (case-insensitive) |
| `{{chain.<alias>.response.status}}` / `.statusText` | status code / status line |
| `{{chain.<alias>.response.body}}` / `.text` | the raw response body |
| `{{chain.<alias>.request.url}}` / `.method` / `.body` | what the predecessor was sent |

Only **scalar** leaves resolve (string / number / bool; `null` → empty). They
resolve at Send time, *after* the chain runs — so the URL-bar preview leaves them
untouched, and an unresolvable chain path (wrong alias or missing field) fails
the Send with an `unresolved variables` error. XML response bodies aren't
navigable from templates — use a script for those.

### Use a chained result in a script

For anything beyond substitution (conditionals, reshaping, combining values),
read `chain.<alias>` in the dependent request's **Scripts → Pre-request** hook —
the same shape as the template paths, plus lazily parsed `json` / `xml`:

```js
// Mutate this request directly:
request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;

// …or stash a value as a session variable, then use {{token}} anywhere:
helena.env.set("token", chain.login.response.json.token);
```

`helena.env.set` writes an in-memory **session overlay** (never saved to your env
file) that layers over the active environment for the rest of the process. You
can also push from the other end: give the chained request a **Post-response**
hook that runs `helena.env.set("token", response.json.token)`, and the dependent
request just uses `{{token}}`.

The full scripting surface — the `helena.*` API and the `request` / `response`
object shapes — is documented in
[internal/scripting/README.md](internal/scripting/README.md).

## Build from source

Requirements:

- Go 1.23+ to build. The exact build toolchain is pinned in `go.mod`
  (`go 1.26` language version, `toolchain go1.26.4`); `go` auto-selects that
  toolchain, and CI locks it with `GOTOOLCHAIN=local` so it never silently
  drifts.
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

## Privacy

Helena makes **no background network requests** and ships **no telemetry,
analytics, or crash reporting**. The only outbound traffic is what you
explicitly trigger:

- **sending a request** (to the host you typed),
- **fetching an OAuth2 token** (from the token endpoint you configured),
- **importing from a URL** (when you paste one into the importer).

There are no other fixed-host calls anywhere in the codebase. Your
collections, credentials, and settings stay on your local disk.

## Security

Found a vulnerability? Please report it privately — see
[SECURITY.md](SECURITY.md). Do not open a public issue for security problems.

## License

BSD 4-Clause — see [LICENSE](LICENSE).
