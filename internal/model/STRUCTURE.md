# model — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level godoc only. |
| [model.go](model.go) | Core domain types (`Workspace`, `Collection`, `Folder`, `Request`, `Environment`, `Variable`, `Settings`), enum constants (`Method`, `BodyType`, `Theme`), and helpers (`NewID`, `EnabledPairs`, `DefaultSettings`). |
| [auth.go](auth.go) | `Auth` plus its sub-structs (`BasicAuth`, `BearerAuth`, `APIKeyAuth`, `OAuth2Auth`) and the related enums (`AuthType`, `APIKeyPlacement`, `OAuth2Grant`). Applied by [internal/auth](../auth/). |
| [model_test.go](model_test.go) | Unit tests for method/body validation, content-type mapping, `EnabledPairs`, ID uniqueness, a `Collection` JSON round-trip, and `Scripts.IsEmpty`. |

## Type catalog

### `Request` — [model.go:95](model.go#L95)
A single HTTP request as the user defined it.
- `Params` — query-string `KeyValue`s; merged into the URL when sending.
- `Headers` — request-header `KeyValue`s.
- `Body` — see `Body`.
- `Docs` — free-form markdown shown in the request's Docs tab.
- `Auth` — own auth or `Inherit` from parent; see [auth.go](auth.go).
- `Scripts` — pre/post JavaScript hooks; see `Scripts`.
- `Chain` — ordered list of `ChainStep` before-hooks; see `ChainStep`.
- `Variables` — request-scoped variables (#82): the highest static resolver scope (above environment and collection; only the script overlay wins). Applied only when this request is sent.

### `ChainStep` — [model.go](model.go)
Names another request to execute before this one and binds the result to an alias the request's scripts can read via `chain.<alias>`. Used by [internal/chain](../chain/).
- `Alias` — script-visible name; must be unique within the parent's `Chain`.
- `Request` — slash-separated name path into the same collection (e.g. `"Auth/Login"`). Case-sensitive on the display name.

### `Scripts` — [model.go](model.go)
Per-request JavaScript bodies the [internal/scripting](../scripting/) runtime executes around Send. Both fields are raw ECMAScript source; an empty string disables that hook.
- `PreRequest` — runs before `httpclient.Build` so the script can mutate URL / method / body / headers / params and write to the session env overlay.
- `PostResponse` — runs after the response body is read; canonical use is `helena.env.set("TOKEN", response.json.token)`.
- `IsEmpty` — returns true when neither hook has any non-whitespace content; the UI Send pipeline uses it to short-circuit the runtime entirely.

### `Body` — [model.go:88](model.go#L88)
A request body.
- `Type` — selects how `Content`/`Form`/`FilePath` is interpreted (see `BodyType`).
- `Content` — raw text used for `json`/`xml`/`text` bodies.
- `Form` — field list used for `form-urlencoded` and `multipart-form` bodies.
- `FilePath` / `ContentType` — back the `file` body type (#24): the request sends the exact bytes of the file at `FilePath`, advertised as `ContentType` (defaulting to `application/octet-stream`). Empty for the other body types.

### `KeyValue` — [model.go:80](model.go#L80)
A toggleable key/value pair shared by headers, query params, and form fields.
- `Enabled` — when false, the pair is kept (so the user can re-enable it) but skipped at send time. See `EnabledPairs`.
- `Description` — optional inline note; persisted but not sent.

### `Folder` — [model.go:107](model.go#L107)
A tree node grouping requests and sub-folders inside a collection. Recursive: a folder may contain folders.
- `Auth` — applies to every descendant whose own `Auth` is `Inherit`.

### `Variable` — [model.go:115](model.go#L115)
An environment variable.
- `Enabled` — when false, ignored during `{{var}}` resolution.
- `Secret` — flag indicating the value should be masked in the UI; the value itself is still stored in plain text in YAML.

### `Environment` — [model.go:123](model.go#L123)
A named set of variables (e.g. `Local`, `Staging`, `Prod`) selectable per collection.

### `Collection` — [model.go:130](model.go#L130)
A root tree.
- `Folders` / `Requests` — children at the top level.
- `Environments` — environments scoped to this collection.
- `Variables` — collection-level variables (#80): a resolver scope below the environment (an environment value of the same name wins). Always applied, unlike the selectable `Environments`.
- `Auth` — outermost ancestor in the auth-inheritance walk; collection roots default to `None` rather than `Inherit` (no parent to inherit from).

### `Auth` — [auth.go](auth.go)
Authentication configuration carried on `Request`, `Folder`, and `Collection`.
- `Type` — selects which sub-struct is in use; see `AuthType` constants.
- `Basic` / `Bearer` / `APIKey` / `OAuth2` — pointer fields; exactly one is non-nil for a concrete auth.
- The zero value (`Type == ""`) is treated as `Inherit` at load time so freshly created requests inherit from their parent without the caller having to set anything.

### `BasicAuth` / `BearerAuth` / `APIKeyAuth` / `OAuth2Auth` — [auth.go](auth.go)
The credential sub-structs. Every string field runs through the `{{var}}` resolver at send time via [internal/auth.ResolveValues](../auth/auth.go).
- `BasicAuth` — `Username`, `Password`. Encoded as `Authorization: Basic <base64>`.
- `BearerAuth` — `Token`. Encoded as `Authorization: Bearer <token>`.
- `APIKeyAuth` — `Name`, `Value`, `Placement` (`header` or `query`).
- `OAuth2Auth` — `Grant`, `TokenURL`, `AuthURL`, `ClientID`, `ClientSecret`, `Scope`, `RedirectURI`, `UsePKCE`, `Audience`. Apply is stubbed until task 7.1c.

### `AuthType` / `APIKeyPlacement` / `OAuth2Grant` — [auth.go](auth.go)
Typed string enums. `AuthType` covers `none`, `inherit`, `basic`, `bearer`, `apikey`, `oauth2`. `APIKeyPlacement` is `header` or `query`. `OAuth2Grant` is `client_credentials` or `authorization_code`.

### `Workspace` — [model.go:139](model.go#L139)
A bag of collections; only metadata lives here — actual collections are referenced by path in the persisted config.

### `Settings` — [model.go:156](model.go#L156)
App-wide preferences.
- `InsecureSkipVerify` — when true, the HTTP client trusts invalid/self-signed TLS certificates.
- `CORSWarning` — when true, the UI shows a CORS advisory note on responses.
- `FollowRedirects` — whether the HTTP client follows 3xx automatically.
- `TimeoutSeconds` — per-request timeout.
- `Theme` — see `Theme`.

### `Method` / `BodyType` / `Theme` — [model.go:9](model.go#L9), [model.go:36](model.go#L36), [model.go:146](model.go#L146)
Typed string enums; the package-level constants (`GET`, `BodyJSON`, `ThemeDark`, …) and the `Methods`/`BodyTypes` slices enumerate the legal values in display order.
