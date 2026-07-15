# Helena User Guide

Helena is a free, open-source, no-cloud API client. This guide covers the
day-to-day workflow. For building from source, see the
[README](../README.md); for the design, see [AGENTS.md](../AGENTS.md).

## Contents

- [Getting started](#getting-started)
- [Collections, folders, requests](#collections-folders-requests)
- [Sending a request](#sending-a-request)
- [Real-time (SSE & WebSocket)](#real-time-sse-websocket)
- [Query, headers, body](#query-headers-body)
- [Authentication](#authentication)
- [Environments & variables](#environments-variables)
- [Request chaining](#request-chaining)
- [Scripting & assertions](#scripting-assertions)
- [Cookies](#cookies)
- [Request history](#request-history)
- [Import & export](#import-export)
- [Headless runs](#headless-runs)
- [Settings](#settings)
- [Privacy & secrets](#privacy-secrets)
- [Diagnostics](#diagnostics)
- [Keyboard shortcuts](#keyboard-shortcuts)

## Getting started

On first launch you'll see an empty-state panel. Three ways to begin:

- **Open collection…** — point Helena at an existing OpenCollection folder.
- **Import…** — build a collection from an OpenAPI/Swagger/WSDL/Postman spec
  (file or URL).
- **Load sample** — materialize the bundled `httpbin` sample and explore it
  without any setup.

The same actions live on the sidebar toolbar. In-app help is under the **?**
button (Getting started, shortcuts, History, this guide, About).

## Collections, folders, requests

The sidebar is a tree of collections → folders → requests. Toolbar actions:
new collection, open, import; the per-node actions (rename, duplicate, delete,
add) appear when a node is selected. Drag a request or folder to reorder it, or
drag a collection to reorder collections. Everything is stored as plain YAML on
disk in the OpenCollection layout — Helena only records the folder paths in its
config; the collection contents live in their own directory.

## Sending a request

Pick a request (or just type a method + URL), then **Send** (Mod+Enter). The
response pane shows the body (auto-detected JSON/XML/HTML, with fold +
highlight + search), headers, and a status line with size and timing. A
long-running send can be **aborted** with the same button. Failures surface in
a dismissible banner above the response, so they aren't lost when the status
line updates.

## Real-time (SSE & WebSocket)

- **WebSocket** — enter a `ws://` or `wss://` URL and press **Send**: instead
  of an HTTP request, Helena opens a live session with a two-way transcript —
  type a message and press Send to write, received messages append as they
  arrive. Pings are answered automatically and fragmented messages are
  reassembled.
- **SSE** — on a `text/event-stream` endpoint, press the dedicated **Stream
  (SSE)** toolbar button (not Send): events append to the response body live,
  and the button doubles as **Stop** while the stream is open.

See the [Real-time guide](guide/realtime.md) for details.

## Query, path, headers, body

- **Query** and the URL field are two views of one thing — edit either and the
  other follows. Disabled query rows (unchecked) are kept even though the URL
  can't express them.
- **Path** — fills the single-brace `{name}` placeholders in the URL's path
  (e.g. `{{base_url}}/users/{id}`). The tab lists one row per placeholder,
  derived live from the URL, so you set a value instead of hand-editing the raw
  URL; the resolved preview under the URL shows the real target and flags any
  placeholder still unfilled. (This is distinct from `{{name}}` variables —
  double braces resolve from environments/collections, single braces are
  per-request path values.)
- **Headers** — enable/disable per row.
- **Body** — None / raw (JSON, XML, text) with validate + format, GraphQL
  (query + variables, sent as a JSON envelope), a structured form editor for
  `form-urlencoded` / `multipart/form-data`, or the raw bytes of a **file** on
  disk.

## Authentication

Per request (or inherited from the folder/collection): None, Basic, Digest,
NTLM, Bearer, API key (header or query), OAuth 1.0a, OAuth 2.0
(client-credentials and authorization-code, with PKCE), WS-Security (WSSE),
or AWS Signature v4. Every scheme's secret fields — passwords, tokens,
client/consumer secrets, the AWS secret key — are masked in the editor.
`Inherit` walks up the folder → collection chain. See the
[Authentication guide](guide/auth.md) for per-scheme details.

## Environments & variables

`{{name}}` templates in URLs, headers, params, body, and auth resolve against
the **active environment**. Use the toolbar icon buttons next to the
environment dropdown: the **gears** button manages environments (create /
rename / delete / switch) and the **table-list** button edits the active
environment's variables — an editable key/value list (one row per variable;
the **+** button appends a row, the row checkbox enables/disables a variable,
the trash icon removes it). A **Secret** variable shows a masked, read-only
value until you tick **Reveal secret values**. Variables compose — a variable's
value may reference another `{{var}}`. A `{{?Name}}` **prompt variable** isn't
resolved from any scope: Helena asks for its value in a dialog at Send time.

## Request chaining

A request can declare *chain steps* that run before it, exposing each
predecessor's parsed response as `{{alias.body.field}}` (and to scripts as
`chain.<alias>`). Helena resolves the graph recursively with per-request alias
scope and cycle detection. See the README's "Request chaining" section for
worked examples.

## Scripting & assertions

Each request can carry a **pre-request** script (mutates method / URL / headers
/ params / body before the request is built) and a **post-response** script
(reads the parsed response, writes values into the environment overlay via
`helena.env.set`, and declares checks with `test()`/`expect()`). Scripts run in
a sandboxed JS runtime with a short timeout.

Prefer no code? The **Assertions** tab holds (source, operator, expected) rows
— e.g. `res.status` equals `200`, or `res.json.user.id` exists — evaluated
after Send and reported in the Scripts console alongside the `test()` results.
See the [Scripting & assertions guide](guide/scripting.md).

## Cookies

Helena keeps a **cookie jar** for the running session. When a response sends
`Set-Cookie`, the cookie is stored and automatically replayed on later matching
requests — so a login request followed by a call to a protected endpoint just
works, including within a chain run and across separate sends while the app is
open. Cookies match by domain, path, and the `Secure` flag, exactly as a browser
would.

Open the jar with the **cookie** button in the top bar to view, add, edit, or
delete cookies, or clear them all. A cookie you add by hand defaults to its exact
host; tick **Send to subdomains** to widen it to sub-domains. Cookies set
explicitly via a `Cookie` request header are still sent — jar cookies are added
alongside them.

The jar is **in-memory only**: it is never written to disk (so session tokens
can't leak into a file) and is emptied when you quit Helena.

## Request history

**?** → **History** lists past sends newest-first. Select an entry to
**Restore** it (reopens the request in a tab), **Resend** it, or **Clear** the
whole list. Snapshots are secret-scrubbed before they are recorded, so
`history.yml` never stores a credential.

## Import & export

- **Import** an OpenAPI 3 / Swagger 2 / WSDL / Postman (v2.x) document (file
  or URL) into a new collection, or paste a **cURL command** to build a single
  request.
- **Export** the current request as a ready-to-run **cURL** or **wget**
  command, or a **JavaScript fetch**, **Python requests**, or **Go net/http**
  snippet (read-only, copyable).

## Headless runs

`helena run <collection-dir> [--env NAME] [--format text|json|junit]
[--folder PATH]` executes every request in a collection (or a single folder)
without opening the UI — chains, scripts, and assertions included — and exits
non-zero when any request errors or any check fails, so it slots straight into
CI. Flags may come before or after the directory. `{{?Name}}` prompt variables
can't be asked headlessly, so a request that uses one fails the run.

## Settings

Theme (System/Light/Dark), allow-invalid-TLS (with a risk caption — it affects
**all** requests and imports), CORS advisory, follow-redirects, request
timeout, the max response size buffered into memory, and a **Global
variables** editor (app-wide, lowest-precedence variable scope). Settings
persist in your OS config directory and carry a schema version so future
upgrades can migrate cleanly.

## Privacy & secrets

Helena makes **no background network requests** and ships **no telemetry**. The
only outbound traffic is what you trigger (sending a request, fetching an
OAuth2 token, importing from a URL, or clicking **Check for updates** in the
status bar). Collections are stored as plain YAML on
your local disk; credential fields (auth secrets, Secret-flagged variables)
are split out on save into a per-collection secrets store under your OS config
directory — never into the collection folder, so a git-tracked collection
can't leak them — and merged back on load. The store itself is plaintext YAML;
treat it like any secrets file. Secret values are masked in the UI and
redacted from logs and error messages.

## Diagnostics

Run `helena --version` to print the build (tag + commit for releases). For bug
reports, run with `helena --verbose` and/or `--log-file PATH` (or set
`HELENA_LOG`) to capture a log — credentials are redacted, so the log is safe
to attach.

## Keyboard shortcuts

Press **F1** (or **?** → Keyboard shortcuts) for the full list. Common ones:
**Mod+Enter** send · **Mod+S** save · **Mod+Z** undo last delete · **Mod+E**
environments · **Mod+,** settings.
