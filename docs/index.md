# Helena 🐱

A super-lightweight, cross-platform **API client** — a native alternative to
Postman and Bruno, built with **Go + [Fyne](https://fyne.io)**. One
self-contained binary, no Electron.

[Get started →](USER_GUIDE.md){ .md-button .md-button--primary }
[View on GitHub →](https://github.com/ideaconnect/helena){ .md-button }

## Why Helena

- **Plain-text, version-controllable storage.** Workspaces, collections,
  folders, and requests are stored as Open Collection YAML on disk — diff and
  commit them like any other source. Credentials are kept **out** of that YAML
  (externalized to a store under your config dir), so a committed collection
  carries no cleartext secret.
- **Variables everywhere.** `{{variable}}` resolution in the URL, query, headers,
  and body, across a layered scope chain (global → `.env` → collection →
  environment → folder → request), with a live resolved-URL preview.
- **A real request builder.** Method, URL, query, headers, and bodies —
  JSON / XML / text / **GraphQL** / form-urlencoded / multipart / file — with
  Validate + Format for JSON and XML.
- **Native, not a browser.** Helena always sends the request and flags responses
  a browser's CORS policy *would* have blocked, instead of refusing to send.

## Highlights

<div class="grid cards" markdown>

- :material-lock: **Nine authentication schemes** — Basic, Bearer, API Key,
  OAuth 2.0 (client-credentials & auth-code + PKCE), OAuth 1.0a, WSSE, AWS
  Signature v4, HTTP Digest, and NTLM. See [Authentication](guide/auth.md).

- :material-broadcast: **Real-time** — stream Server-Sent Events and open
  bidirectional WebSocket sessions, both hand-rolled on the standard library.
  See [Real-time](guide/realtime.md).

- :material-language-javascript: **Scripting & assertions** — pre/post
  JavaScript hooks with a curated `helena.*` API (env, interpolate,
  sendRequest, cookies, hashing, …), a `test()`/`expect()` framework, and
  declarative no-code assertions. See [Scripting](guide/scripting.md).

- :material-console: **Chaining & a headless runner** — run other requests
  first and feed their results forward, and run a whole collection from the CLI
  with `helena run`.

</div>

## Install

Grab a binary from the [releases page](https://github.com/ideaconnect/helena/releases),
or build from source:

```bash
git clone https://github.com/ideaconnect/helena
cd helena
go build -o helena ./cmd/helena
```

See [Getting started](USER_GUIDE.md) for a first request and
[Packaging](PACKAGING.md) for platform builds.

!!! note "Documentation"
    This site is generated from the Markdown in the repo's `docs/` directory.
    Deep, per-package design notes live alongside the code in each module's
    `README.md` / `STRUCTURE.md` / `WORKFLOW.md`.
