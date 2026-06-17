# Helena User Guide

Helena is a free, open-source, no-cloud API client. This guide covers the
day-to-day workflow. For building from source, see the
[README](../README.md); for the design, see [AGENTS.md](../AGENTS.md).

## Contents

- [Getting started](#getting-started)
- [Collections, folders, requests](#collections-folders-requests)
- [Sending a request](#sending-a-request)
- [Query, headers, body](#query-headers-body)
- [Authentication](#authentication)
- [Environments & variables](#environments--variables)
- [Request chaining](#request-chaining)
- [Scripting (pre/post)](#scripting-prepost)
- [Import & export](#import--export)
- [Settings](#settings)
- [Privacy & secrets](#privacy--secrets)
- [Diagnostics](#diagnostics)
- [Keyboard shortcuts](#keyboard-shortcuts)

## Getting started

On first launch you'll see an empty-state panel. Three ways to begin:

- **Open collection…** — point Helena at an existing OpenCollection folder.
- **Import…** — build a collection from an OpenAPI/Swagger/WSDL spec (file or
  URL).
- **Load sample** — materialize the bundled `httpbin` sample and explore it
  without any setup.

The same actions live on the sidebar toolbar. In-app help is under the **?**
button (Getting started, shortcuts, this guide, About).

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

## Query, headers, body

- **Query** and the URL field are two views of one thing — edit either and the
  other follows. Disabled query rows (unchecked) are kept even though the URL
  can't express them.
- **Headers** — enable/disable per row.
- **Body** — None / raw (JSON, XML, text) with validate + format, or a
  structured form editor for `form-urlencoded` / `multipart/form-data`.

## Authentication

Per request (or inherited from the folder/collection): None, Basic, Bearer,
API key (header or query), or OAuth2 (client-credentials and
authorization-code, with PKCE). Secret fields — Basic password, Bearer token,
API-key value, OAuth2 client secret — are masked in the editor. `Inherit`
walks up the folder → collection chain.

## Environments & variables

`{{name}}` templates in URLs, headers, params, body, and auth resolve against
the **active environment**. Manage environments with **Manage…** (create /
rename / delete / switch) and edit variables with **Variables…** — an editable
key/value list (one row per variable; **Add variable** to append, the row
checkbox enables/disables a variable, the trash icon removes it). A **Secret**
variable shows a masked, read-only value until you tick **Reveal secret
values**. Variables compose — a variable's value may reference another `{{var}}`.

## Request chaining

A request can declare *chain steps* that run before it, exposing each
predecessor's parsed response as `{{alias.body.field}}` (and to scripts as
`chain.<alias>`). Helena resolves the graph recursively with per-request alias
scope and cycle detection. See the README's "Request chaining" section for
worked examples.

## Scripting (pre/post)

Each request can carry a **pre-request** script (mutates method / URL / headers
/ params / body before the request is built) and a **post-response** script
(reads the parsed response, writes values into the environment overlay via
`helena.env.set`). Scripts run in a sandboxed JS runtime with a short timeout.

## Import & export

- **Import** an OpenAPI 3 / Swagger 2 / WSDL document (file or URL) into a new
  collection.
- **Export** the current request as a ready-to-run **cURL** or **wget**
  command (read-only snippet you can copy).

## Settings

Theme (System/Light/Dark), allow-invalid-TLS (with a risk caption — it affects
**all** requests and imports), CORS advisory, follow-redirects, request
timeout, and the max response size buffered into memory. Settings persist in
your OS config directory and carry a schema version so future upgrades can
migrate cleanly.

## Privacy & secrets

Helena makes **no background network requests** and ships **no telemetry**. The
only outbound traffic is what you trigger (sending a request, fetching an
OAuth2 token, importing from a URL). Collections and credentials are stored as
plaintext YAML on your local disk today — treat those files like any secrets
file. Secret values are masked in the UI and redacted from logs and error
messages.

## Diagnostics

Run `helena --version` to print the build (tag + commit for releases). For bug
reports, run with `helena --verbose` and/or `--log-file PATH` (or set
`HELENA_LOG`) to capture a log — credentials are redacted, so the log is safe
to attach.

## Keyboard shortcuts

Press **F1** (or **?** → Keyboard shortcuts) for the full list. Common ones:
**Mod+Enter** send · **Mod+S** save · **Mod+E** environments · **Mod+,**
settings.
