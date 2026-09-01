# importer

`importer` converts external API descriptions into Helena's native `model.Collection`. It accepts OpenAPI 3 and Swagger 2 (JSON or YAML, converted in-memory via `openapi2conv`), WSDL 1.1 (parsed with `encoding/xml`), Postman Collection v2.x (JSON), and URL-fetched payloads of any of the above.

The package exposes one auto-detecting entry point, `From`, plus the format-specific parsers it dispatches to. Auto-detection is intentionally tiny: a Postman collection (an `info` object plus an `item` array, and no `openapi`/`swagger` key) is recognized first; otherwise the *first non-whitespace byte* decides — `<` means WSDL/XML, anything else is treated as OpenAPI/Swagger and disambiguated later by the presence of an `openapi` or `swagger` top-level key.

OpenAPI servers, tags, parameters and request bodies are mapped to Helena's URL/base-URL variable, folders, header/param rows and request bodies respectively. WSDL operations become POST requests pre-loaded with a placeholder SOAP envelope; Helena does not try to materialize a fully typed payload.

## Public API

- `From(data []byte) (model.Collection, error)` — auto-detecting dispatcher; routes Postman collections to `FromPostman`, XML to `FromWSDL`, everything else to `FromOpenAPI`.
- `FromOpenAPI(data []byte) (model.Collection, error)` — parses OpenAPI 3 or Swagger 2 (auto-detected by `openapi`/`swagger` key); accepts JSON or YAML bytes. Every operation a path item declares is imported, including OpenAPI 3.2's fixed `query` field and its `additionalOperations` map of custom method names — those yield requests whose method (`QUERY`, `PURGE`, …) is outside `model.Methods`, so it sends and round-trips normally but the method picker's dropdown does not list it.
- `FromPostman(data []byte) (model.Collection, error)` — parses a Postman Collection v2.x JSON document. Folders, requests, headers (with the `disabled` flag), query params, request bodies (`raw` typed by `options.raw.language`, `urlencoded`, `formdata`, `graphql` stored as a JSON body), and the bearer/basic/apikey/noauth schemes are mapped; events/scripts, response examples and file bodies are dropped rather than failing the import. A URL given as an object with no `raw` is reconstructed from its host + path parts.
- `FromWSDL(data []byte) (model.Collection, error)` — parses a WSDL 1.1 document into one POST per binding operation.
- `FromURL(url string, settings model.Settings) (model.Collection, error)` — fetches a spec over HTTP(S), honoring `InsecureSkipVerify` and `TimeoutSeconds`, then forwards the body through `From`.
- `FromCurl(command string) (model.Request, error)` — parses a copy-pasted `curl` command line into a single `model.Request` (method, URL, headers, query, body, basic auth). It returns a `Request`, not a `Collection`, since a curl command is one request; the UI opens it in a scratch tab. Handles `-X/-H/-d/--data*/-F/-u/-b/-A/-e/-G/--url` + a positional URL, shell quoting, and `\` line continuations; unknown flags are skipped.

## Dependencies

- [`github.com/getkin/kin-openapi/openapi3`](https://pkg.go.dev/github.com/getkin/kin-openapi/openapi3) — OpenAPI 3 model and loader.
- [`github.com/getkin/kin-openapi/openapi2`](https://pkg.go.dev/github.com/getkin/kin-openapi/openapi2) — Swagger 2 model.
- [`github.com/getkin/kin-openapi/openapi2conv`](https://pkg.go.dev/github.com/getkin/kin-openapi/openapi2conv) — in-memory Swagger 2 -> OpenAPI 3 conversion.
- [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML decoding before re-marshaling as JSON.
- `encoding/xml` — WSDL parsing.
- `encoding/json`, `fmt`, `strings`, `net/http`, `io`, `crypto/tls`, `time` — standard library.
- [`github.com/idct/helena/internal/model`](../model) — `Collection`, `Folder`, `Request`, `Environment`, `Variable`, `KeyValue`, `Body`, `BodyType`, `Method`, `Auth`.
