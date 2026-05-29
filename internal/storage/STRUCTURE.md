# storage — structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level doc comment. |
| [opencollection.go](opencollection.go) | DTO structs that mirror the OpenCollection YAML schema, plus the small `model` ↔ DTO converters. |
| [store.go](store.go) | The `Save`/`Load` entry points and the directory walker, including the Extra round-trip and orphan sweep. |
| [storage_test.go](storage_test.go) | Round-trip, key-naming and docs-key tests. |
| [storage_extras_test.go](storage_extras_test.go) | Extra round-trip and orphan sweep tests against hand-written YAML. |
| [storage_scripts_test.go](storage_scripts_test.go) | Scripts round-trip — on-disk key names, empty-Scripts omission, and `scripts.Extra` survival across a load → save cycle. |

## DTO layer

All DTOs live in [opencollection.go](opencollection.go) and carry an
`Extra map[string]yaml.Node` field tagged `yaml:",inline"` so any YAML keys
the struct does not name go into `Extra` on unmarshal and are inlined back into
the output on marshal. This is the heart of the lossless round-trip.

| DTO | Mirrors | Domain counterpart |
| --- | --- | --- |
| `ocInfo` | the `info:` block on every file (name, type, seq, tags + Extra) | the `Name` and `Type` discrimination on each model type |
| `ocKV` | a header entry (name/value/disabled + Extra) | `model.KeyValue` (Key/Value/Enabled, with Disabled inverted) |
| `ocParam` | a query/path parameter (name/value/type/disabled + Extra) | `model.KeyValue` used for `model.Request.Params` |
| `ocBody` | a request body (`type`, `data` + Extra) | `model.Body` (`Type`, `Content`) |
| `ocHTTP` | the `http:` block of a request (method, url, headers, params, body, auth + Extra) | the HTTP-level fields of `model.Request` |
| `ocRequestFile` | one request `.yml` (info + http + docs + scripts + Extra) | `model.Request` |
| `ocScripts` | the per-request `scripts:` block (`preRequest`, `postResponse`, + Extra) | `model.Scripts` |
| `ocFolderFile` | one `folder.yml` (info + auth + Extra) | `model.Folder` (name + auth; folders/requests are read from the surrounding directory) |
| `ocCollectionFile` | the root `opencollection.yml` (info + auth + Extra) | the top-level `model.Collection` (name + auth; the rest comes from the directory) |
| `ocAuth` | the `auth:` block on requests / folders / collections (`type`, one sub-block, + Extra) | `model.Auth` |
| `ocAuthBasic` / `ocAuthBearer` / `ocAuthAPIKey` / `ocAuthOAuth2` | the credential sub-blocks under `auth.<type>` | `model.BasicAuth` / `BearerAuth` / `APIKeyAuth` / `OAuth2Auth` |
| `ocEnvVar` | one entry in `environments/*.yml` `vars:` (name/value/disabled/secret + Extra) | `model.Variable` |
| `ocEnvironmentFile` | one `environments/*.yml` (info + vars + Extra) | `model.Environment` |

### DTO ↔ model converters

| Function | Direction |
| --- | --- |
| [`requestToFile`](opencollection.go) | `model.Request` → `ocRequestFile` for marshalling. |
| [`fileToRequest`](opencollection.go) | `ocRequestFile` → `model.Request`, assigning a fresh ID. Missing auth defaults to `AuthInherit`. |
| [`envToFile`](opencollection.go) | `model.Environment` → `ocEnvironmentFile`. |
| [`fileToEnv`](opencollection.go) | `ocEnvironmentFile` → `model.Environment`, assigning a fresh ID. |
| [`authToFile`](opencollection.go) | `model.Auth` → `*ocAuth`. Returns nil for `""` / `Inherit` so the YAML stays clean for collections that never configured auth. |
| [`fileToAuth`](opencollection.go) | `*ocAuth` → `model.Auth`. Nil input is treated as `AuthInherit` for request/folder callers; the collection-root load path explicitly substitutes `AuthNone` instead. |
| [`scriptsToFile`](opencollection.go) | `model.Scripts` → `*ocScripts`. Returns nil when both hooks are empty so the YAML stays clean for non-scripted requests. |
| [`fileToScripts`](opencollection.go) | `*ocScripts` → `model.Scripts`. Nil input produces the zero `Scripts` value (both hooks empty). |

The model's `KeyValue.Enabled` is flipped to the DTO's `Disabled` and back so
the on-disk representation matches OpenCollection's convention of recording
the negated form.

## Walker (store.go)

| Symbol | Role |
| --- | --- |
| `Save` | Public entry, writes the root file, environments and items, then sweeps. |
| `Load` | Public entry, reads the root file, environments and items. |
| `saveItems` | Recursive: writes one container (collection root or folder) and recurses into subfolders. |
| `loadItems` | Recursive: reads one container's `.yml` request files and folder subdirectories. |
| `loadEnvironments` | Reads every `.yml` under `environments/`, sorted by `info.seq`. |
| `sweepDir` | Removes `.yml` files and folder-style subdirectories not produced by this save (leaves user files / the env dir alone). |
| `readRequestFile`, `readFolderFile`, `readCollectionFile`, `readEnvFile` | Load a single file as its DTO, preserving Extra fields. |
| `writeYAML` | Marshal-and-write helper. |
| `slug` | Display name → filesystem-friendly base name, with a fallback when nothing usable remains. |
| `uniqueName` | Resolves slug collisions by appending `-2`, `-3`, … |

## Constants

Defined in [store.go](store.go):

- `collectionFile = "opencollection.yml"`
- `folderFile     = "folder.yml"`
- `environmentsDir = "environments"`
- `ymlExt         = ".yml"`
