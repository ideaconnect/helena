---
layout: page
title: Features
eyebrow: What Helena does
lead: A full-featured API client in one small native binary - without the browser engine or the lock-in.
description: Helena's features - auth, scripting, real-time, chaining, the runner, import/export, and diffable storage.
---

## Request builder

Method, URL, query params, headers, and a body that can be **JSON, XML, text,
GraphQL, form-urlencoded, multipart, or a raw file**. JSON and XML get Validate
and Format buttons. The response viewer shows raw, pretty JSON, pretty XML, and
headers, with a status line like `200 OK · 1.2 KB · 87 ms` and a CORS advisory
when a browser *would* have blocked the response (Helena, being native, sends it
anyway).

## Variables &amp; environments

{% raw %}`{{variable}}`{% endraw %} resolution everywhere - URL, query, headers, and body - with a
live preview of the resolved URL. Variables resolve through a layered scope
chain so the most specific value wins:

`global → collection .env → collection → environment → folder → request → script overlay`

Plus dynamic values like {% raw %}`{{$guid}}`, `{{$timestamp}}`{% endraw %}, and ask-at-send
{% raw %}`{{?Name}}`{% endraw %} prompt variables.

## Authentication

Nine schemes, all with {% raw %}`{{var}}`{% endraw %} substitution and credentials kept out of the
committed YAML:

Basic · Bearer · API Key · **OAuth 2.0** (client-credentials and
authorization-code + PKCE, with token caching) · **OAuth 1.0a** (HMAC-SHA1) ·
**WSSE** · **AWS Signature v4** · **HTTP Digest** (challenge/response) ·
**NTLM** (the multi-round Windows handshake).

[Full authentication guide →]({{ site.repo }}/blob/main/docs/guide/auth.md)

## Scripting, tests &amp; assertions

Pre/post **JavaScript** hooks (via the pure-Go goja engine) with a curated
`helena.*` API - environment access, `interpolate`, `sendRequest`, cookie
reads, hashing, base64, uuid, sleep, and headless runner control. Verify
responses with a scripted `test()`/`expect()` framework or the no-code
**Assertions** tab.

## Request chaining

A request can declare other requests to run **first**; their results are bound
as `chain.<alias>` for the leaf's scripts and templates - so you can log in,
then use the token, in a single Send.

## Real-time - SSE &amp; WebSocket

Stream **Server-Sent Events** straight into the response view, or open a
**WebSocket** session (`ws://` / `wss://`) with a live two-way transcript. Both
protocols are hand-rolled on the Go standard library - no third-party network
dependency.

## Collections &amp; the headless runner

Organize requests into workspaces, collections, and folders. Run a whole
collection from the GUI, or **headlessly from the CLI** with `helena run` for CI
- same resolution, scripts, and assertions as an interactive Send.

## Import &amp; export

Import **OpenAPI 3 / Swagger 2 / WSDL / Postman** from a file or URL. Export any
request to **cURL, wget, JavaScript fetch, Python requests, or Go net/http**, or
paste a cURL command to build a request.

## Storage &amp; privacy

Everything is plain **Open Collection YAML** on disk - diff and merge it like
source code. Auth secrets and Secret-flagged variables are **externalized** to a
store under your OS config dir (outside the repo), so a committed collection
carries no cleartext credential. The session env overlay never touches disk.

## Native niceties

Light / dark / system theme, a configurable request timeout and max response
size, an invalid-SSL toggle, a cookie jar with a viewer, and keyboard shortcuts
(Mod+Enter to send, Mod+S to save, …).
