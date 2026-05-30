# testdata/

Shared fixtures referenced by tests across multiple packages. Files
specific to a single package's tests live under that package's own
`testdata/` (Go convention). Anything here is expected to be reused
by ≥ 2 packages or by the `features/` + `integration/` suites.

## Layout

| Path | Used by | Notes |
| --- | --- | --- |
| `openapi/minimal.yaml` | importer tests, godog import_export | OpenAPI 3.0 with one path. |
| `openapi/complex.yaml` | importer tests, godog import_export, integration | OpenAPI 3.1 with tag folders, body examples, query / header / path params, multi-server. |
| `openapi/broken.yaml` | importer negative tests | Intentionally malformed YAML — verifies the parser surfaces a clear error rather than panicking. |
| `swagger/basic.yaml` | importer tests | Swagger 2.0 minimal. |
| `swagger/parameters.yaml` | importer tests | Swagger 2.0 covering `in: query / header / path / formData`. |
| `wsdl/rpc.wsdl` | wsdl importer tests | RPC-style SOAP binding. |
| `wsdl/document.wsdl` | wsdl importer tests | Document/literal style with embedded schema. |
| `collections/minimal/` | storage / session / integration | Smallest legal Open Collection (root + one request). |
| `collections/complex/` | storage / session / integration | Folder with Bearer (var-templated) + Inherit child + chain ref + scripts. |
| `collections/extras/` | storage Extras-roundtrip tests | Hand-authored with unknown fields under collection / info / headers / params / scripts / chain — verifies AGENTS invariant 1. |
| `responses/users.json` | scripting tests, response-format tests | Realistic paginated JSON body. |
| `responses/feed.xml` | scripting xml tests, response-format tests | Atom feed; multiple `<entry>` children exercise the xml-array binding. |
| `responses/login.json` | auth / scripting tests | OAuth-flavored token response. |

## Adding new fixtures

- If only one package uses it, prefer that package's own `testdata/`.
- If two or more packages would use it, put it here and add a row above.
- Filenames stay descriptive (no `test1.yaml` / `data.json`). The file's purpose should be clear from the path.
- Keep fixtures small (≤ 100 lines preferred) and self-contained.
