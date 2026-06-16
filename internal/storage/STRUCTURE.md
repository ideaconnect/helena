# storage — structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level doc comment. |
| [opencollection.go](opencollection.go) | DTO structs that mirror the OpenCollection YAML schema, plus the small `model` ↔ DTO converters. |
| [store.go](store.go) | The `Save`/`Load` entry points and the directory walker, including the Extra round-trip and orphan sweep. |
| [storage_test.go](storage_test.go) | Round-trip, key-naming and docs-key tests. |
| [storage_extras_test.go](storage_extras_test.go) | Extra round-trip and orphan sweep tests against hand-written YAML. |
| [storage_scripts_test.go](storage_scripts_test.go) | Scripts round-trip — on-disk key names, empty-Scripts omission, `scripts.Extra` survival, and Extra preservation when the user clears both hooks. |
| [storage_chain_test.go](storage_chain_test.go) | Chain round-trip — on-disk key names, empty-Chain omission, and per-entry Extra preservation across a load → save cycle. |

## DTO layer

All DTOs live in [opencollection.go](opencollection.go) and carry an
`Extra map[string]yaml.Node` field tagged `yaml:",inline"` so any YAML keys
the struct does not name go into `Extra` on unmarshal and are inlined back into
the output on marshal. This is the heart of the lossless round-trip.

| DTO | Mirrors | Domain counterpart |
| --- | --- | --- |
| `ocInfo` | the `info:` block on every file (name, **id**, type, seq, tags + Extra) | the `Name`, `ID`, and `Type` discrimination on each model type. `id` is Helena's stable identifier; files without one get a fresh ID on Load that the next Save persists. |
| `ocKV` | a header entry (name/value/disabled + Extra) | `model.KeyValue` (Key/Value/Enabled, with Disabled inverted) |
| `ocParam` | a query/path parameter (name/value/type/disabled + Extra) | `model.KeyValue` used for `model.Request.Params` |
| `ocBody` | a request body (`type`, `data` + Extra) | `model.Body` (`Type`, `Content`) |
| `ocHTTP` | the `http:` block of a request (method, url, headers, params, body, auth + Extra) | the HTTP-level fields of `model.Request` |
| `ocRequestFile` | one request `.yml` (info + http + docs + scripts + chain + Extra) | `model.Request` |
| `ocScripts` | the per-request `scripts:` block (`preRequest`, `postResponse`, + Extra) | `model.Scripts` |
| `ocChainStep` | one entry under `chain:` (`alias`, `request`, `requestId`, + Extra) | `model.ChainStep`. `requestId` pins the ref to the target's persistent `Request.ID` so renames + folder moves don't break the chain. |
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
| [`fileToRequest`](opencollection.go) | `ocRequestFile` → `model.Request`. Preserves `info.id` when present (so chain-step `requestId` pins stay valid across reloads); generates a fresh ID when absent (the next Save persists it). Missing auth defaults to `AuthInherit`. |
| [`envToFile`](opencollection.go) | `model.Environment` → `ocEnvironmentFile`. |
| [`fileToEnv`](opencollection.go) | `ocEnvironmentFile` → `model.Environment`, assigning a fresh ID. |
| [`authToFile`](opencollection.go) | `model.Auth` → `*ocAuth`. Returns nil for `""` / `Inherit` so the YAML stays clean for collections that never configured auth. |
| [`fileToAuth`](opencollection.go) | `*ocAuth` → `model.Auth`. Nil input is treated as `AuthInherit` for request/folder callers; the collection-root load path explicitly substitutes `AuthNone` instead. |
| [`scriptsToFile`](opencollection.go) | `model.Scripts` → `*ocScripts`. Returns nil when both hooks are empty so the YAML stays clean for non-scripted requests. |
| [`fileToScripts`](opencollection.go) | `*ocScripts` → `model.Scripts`. Nil input produces the zero `Scripts` value (both hooks empty). |
| [`chainToFile`](opencollection.go) | `[]model.ChainStep` → `[]ocChainStep`. Returns nil for an empty slice. |
| [`fileToChain`](opencollection.go) | `[]ocChainStep` → `[]model.ChainStep`. Nil-safe; returns nil for an empty slice. |

The model's `KeyValue.Enabled` is flipped to the DTO's `Disabled` and back so
the on-disk representation matches OpenCollection's convention of recording
the negated form.

## Walker (store.go)

| Symbol | Role |
| --- | --- |
| `Save` | Public entry. Stages the whole collection into `<dir>.helena-save` then atomically swaps it into place, so a mid-write failure leaves `dir` untouched (#109). |
| `copyTree` | Recursively copies `dir` into the staging dir so the Extra round-trip can read prior files. |
| `saveInPlace` | The non-atomic write logic (root file, environments, items, sweep) that `Save` runs against the staging dir. |
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
