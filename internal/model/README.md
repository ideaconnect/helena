# model

Storage-agnostic domain types for Helena. The model package defines what a workspace, collection, folder, request, environment, variable, and settings record look like in memory — independent of any serialization (YAML for OpenCollection, JSON for tests/imports/exports).

These types are shared by every other layer: storage marshals them to disk, importers (Postman, OpenAPI, WSDL, URL) construct them, the UI binds against them, the HTTP client reads from them, and the exporter renders them out. Keeping the shape free of layer-specific tags or behavior is what makes the codebase composable.

A few small helpers live here too: `NewID` mints random hex IDs for tree nodes, `EnabledPairs` filters enabled headers/params, and `DefaultSettings` provides the baseline app preferences.

## Public API

### Types
- `Method` — string alias for HTTP method names.
- `BodyType` — string alias for request body encodings.
- `KeyValue` — toggleable key/value pair (used for headers, params, form fields).
- `Body` — request body (type + raw content + form fields).
- `Request` — a single HTTP request definition (id, name, method, URL, headers, params, body, docs, auth).
- `Folder` — recursive group of requests and sub-folders, with its own `Auth` inherited by descendants.
- `Variable` — a single environment variable (enabled, key, value, secret).
- `Environment` — a named bag of `Variable`s.
- `Collection` — root tree of folders, requests, environments, and root-level auth.
- `Auth` — per-request / per-folder / per-collection authentication (None / Inherit / Basic / Bearer / API-Key / OAuth2). Sub-structs: `BasicAuth`, `BearerAuth`, `APIKeyAuth`, `OAuth2Auth`. Enums: `AuthType`, `APIKeyPlacement`, `OAuth2Grant`. Resolved and applied by [internal/auth](../auth/).
- `Workspace` — group of collections under one name.
- `Theme` — UI theme string (`system`/`light`/`dark`).
- `Settings` — app-wide preferences (TLS, redirects, timeout, theme, CORS warning).

### Methods
- `Method.Valid()` — reports whether the value is one of the supported methods.
- `BodyType.Valid()` — reports whether the value is a supported body type.
- `BodyType.ContentType()` — returns the implied `Content-Type` header value, or `""` when none applies.

### Constants
- HTTP methods: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`.
- Body types: `BodyNone`, `BodyJSON`, `BodyXML`, `BodyText`, `BodyForm`, `BodyMultipart`.
- Themes: `ThemeSystem`, `ThemeLight`, `ThemeDark`.
- Auth types: `AuthNone`, `AuthInherit`, `AuthBasic`, `AuthBearer`, `AuthAPIKey`, `AuthOAuth2`.
- API-Key placements: `APIKeyHeader`, `APIKeyQuery`.
- OAuth2 grants: `OAuth2ClientCredentials`, `OAuth2AuthorizationCode`.

### Variables
- `Methods` — display-ordered list of supported HTTP methods.
- `BodyTypes` — display-ordered list of supported body types.

### Functions
- `DefaultSettings()` — returns the baseline `Settings` (30s timeout, follow redirects, CORS warning on, system theme).
- `EnabledPairs(kvs)` — returns only the `KeyValue`s where `Enabled == true`, preserving order.
- `NewID()` — returns a random 128-bit hex string for tagging tree nodes.

## Dependencies

### Internal
None. `model` is a leaf package and is imported by nearly every other internal package.

### External (standard library only)
- `crypto/rand` — entropy source for `NewID`.
- `encoding/hex` — hex encoding for `NewID`.
