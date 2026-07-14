# importer — Workflow

## Auto-detecting Postman vs OpenAPI vs WSDL

`From` ([from.go:7](from.go#L7)) makes two cheap decisions:

1. `looksLikePostman` unmarshals a tiny structural probe. If the document has both an `info` object and an `item` array — and no `openapi`/`swagger` key — it routes to `FromPostman`. (OpenAPI/Swagger docs carry one of those keys, so they never misroute even though they also have `info`.)
2. Otherwise, skip leading whitespace and look at the first byte: `<` means XML → `FromWSDL`; anything else — JSON `{`, YAML `o`/`s`/`#`/`-` — goes to `FromOpenAPI`, which disambiguates Swagger 2 vs OpenAPI 3 by probing for `swagger` / `openapi` keys.

WSDL files always start with `<?xml` or `<definitions>`; OpenAPI documents never do. The sniff is intentionally minimal — there is no MIME table, no content-type check, no full parse-and-retry.

## Postman collection → Helena tree

`FromPostman` ([postman.go:14](postman.go#L14)) decodes the v2.x JSON into the `pm*` structs and walks `item` recursively (`pmAppendItem`): an item with a non-nil `request` is a leaf request, otherwise it is a folder (whose children recurse). Per request it maps:

- **URL**: `request.url` is either a bare string or an object; `pmURL.UnmarshalJSON` accepts both, and `effectiveRaw` rebuilds `host.join('.') + '/' + path.join('/')` when the object omits `raw`. Object `query` entries become `Params` (Postman `disabled` → `Enabled=false`).
- **Headers**: `request.header` → `Headers`, with the same disabled-flag mapping.
- **Body**: `pmConvertBody` switches on `mode` — `raw` (typed by `options.raw.language` → JSON/XML/Text), `urlencoded` → `BodyForm`, `formdata` → `BodyMultipart`, `graphql` → a JSON body carrying `{query, variables}` (Helena has no dedicated GraphQL body type), `file`/unknown → no body.
- **Auth**: `pmConvertAuth` maps `bearer`/`basic`/`apikey`/`noauth` (collection, folder, and request level). Unsupported types (e.g. `oauth2`) fall through to the zero `Auth`, which loads as Inherit.

Postman events/scripts, response examples, and binary file bodies have no Helena home and are dropped rather than failing the import.

## Hoisting OpenAPI server URL to `{{base_url}}`

`convertOAS3` ([openapi.go:107](openapi.go#L107)) inspects `doc.Servers`. It takes the first non-nil server whose `URL` is non-empty **after trailing slashes are trimmed** (`strings.TrimRight(url, "/")`) and hoists it into a single-environment, single-variable structure:

```text
Collection.Environments = [Environment{
    Name: "Default",
    Variables: [Variable{Enabled: true, Key: "base_url", Value: strings.TrimRight(server.URL, "/")}],
}]
```

`buildRequest` ([openapi.go:180](openapi.go#L180)) then uses `hasBaseVar=true` to prefix every request's URL with `{{base_url}}`. Because OpenAPI paths always start with `/` and `base_url` is stored slash-free, the join yields exactly one slash — a server URL like `https://api/` no longer renders `https://api//pets` (issue #181); `buildRequest` also prepends a leading `/` if a spec hands it a path without one, as a belt-and-braces guard. When `doc.Servers` is empty, no environment is created and the request URL is the raw path. This means edits to the environment update every imported request consistently — which is the point of hoisting.

## Mapping OpenAPI `requestBody.Content` example to `model.Body`

For each operation, `buildRequest` iterates `op.RequestBody.Value.Content` in sorted key order and picks the **first** media-type entry it finds (`break` after the first non-nil one):

1. Content-type string -> `model.BodyType` via `bodyTypeFromContentType` ([openapi.go:268](openapi.go#L268)) — substring matches for `json`, `xml`, `form-urlencoded`, `multipart`, `text/`, plus an explicit `*/*` -> `BodyJSON` case (Swagger-2 body params with no `consumes` convert to a `*/*` media type, and JSON is the overwhelming body default), defaulting to `BodyText`.
2. Example body string -> `extractExample` ([openapi.go:291](openapi.go#L291)) which prefers `mt.Example`, then any `mt.Examples[*].Value.Value`, then `mt.Schema.Value.Example`.
3. When `extractExample` returns empty **and** the body type is JSON, `synthesizeJSONBody` ([openapi.go:309](openapi.go#L309)) builds a skeleton body from `mt.Schema` via `sampleForSchema` ([openapi.go:328](openapi.go#L328)) — most real specs describe the body with a `$ref` schema and no inline example, so without this the request would import blank (issue #180). `sampleForSchema` walks the schema (explicit `example`/`default`/`const`/`enum` win; `allOf` merges, `oneOf`/`anyOf` take the first branch; objects recurse over `Properties` skipping `readOnly` fields; arrays wrap one element; strings use format-aware placeholders from `placeholderString`; numbers/booleans use `0`/`false`), bounded by `sampleMaxDepth` and an on-path set that breaks cyclic resolved `$ref`s. Synthesis is JSON-only — emitting JSON into an XML/form/multipart body would be wrong.
4. Structured examples (real or synthesized) are pretty-printed JSON via `formatExample` ([openapi.go:417](openapi.go#L417)); string examples pass through verbatim.

Sorting the media-type keys gives deterministic output — without it, the choice between e.g. `application/json` and `application/xml` would depend on map iteration order.

Parameters are handled separately from bodies: `query` params -> `r.Params`, `header` params -> `r.Headers`, `path` params remain embedded in the URL as `{name}` placeholders. Optional params are imported disabled, required params are imported enabled.

## curl command → request

`FromCurl` ([curl.go](curl.go)) tokenizes the command with `tokenizeShell`
(single/double quotes, `\` escapes, `\`-newline continuations), drops a leading
`curl`, then walks the flags: `-X/--request` → method, `-H/--header` →
headers (also noting `Content-Type`), `-d/--data*`/`--data-urlencode` →
accumulated data, `-F/--form` → multipart fields, `-u/--user` → basic auth,
`-A/-e/-b` → User-Agent/Referer/Cookie headers, `--url` + a positional arg →
URL, `-G/--get` → fold the data into the query. A trailing body is mapped by
`bodyFromData` (Content-Type wins; otherwise JSON `{`/`[` and `k=v&…` form
shapes are sniffed). Method defaults to POST when data/form is present, else
GET. Unknown flags are skipped, consuming a value for the common value-taking
ones (`-o`, `--max-time`, …) so they don't swallow the URL. Returns a single
`model.Request`; the UI opens it in a scratch tab.

## WSDL operation → SOAP envelope template

`FromWSDL` ([wsdl.go:17](wsdl.go#L17)) walks `definitions -> services -> ports -> bindings.operations`, deduplicating by `service.port.operation`. For each operation, `buildSOAPRequest` ([wsdl.go:75](wsdl.go#L75)) emits:

- **Method**: POST.
- **URL**: `port.SoapAddress.Location` (the SOAP endpoint).
- **Headers**:
  - `Content-Type: text/xml; charset=utf-8` (always).
  - `SOAPAction: <op.SoapOperation.SoapAction>` (only when non-empty in the binding).
- **Body**: a placeholder envelope of the form:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
                  xmlns:tns="<targetNamespace>">
  <soapenv:Header/>
  <soapenv:Body>
    <tns:<OperationName>>
      <!-- Fill in operation parameters here. -->
    </tns:<OperationName>>
  </soapenv:Body>
</soapenv:Envelope>
```

When `targetNamespace` is empty the `xmlns:tns` attribute and `tns:` prefix are omitted. The body is `model.BodyXML`. Users are expected to fill in the actual parameter elements — this importer is a starting scaffold, not a schema-aware client generator.

If no operations are discovered (e.g. a WSDL with empty `<definitions>`) the call returns `"no SOAP operations found in WSDL"`.

## Fetching a spec from a URL

`FromURL` ([url.go:17](url.go#L17)) builds an ad-hoc `http.Client` from the user's `model.Settings`:

1. `tls.Config{InsecureSkipVerify: settings.InsecureSkipVerify}` on a fresh `http.Transport`.
2. `http.Client.Timeout = time.Duration(settings.TimeoutSeconds) * time.Second` (zero means no timeout — matching the rest of the app).
3. `client.Get(url)`; non-2xx returns `fmt.Errorf("fetch %s: %s", url, resp.Status)`.
4. Read the entire body with `io.ReadAll`.
5. Pipe the bytes through `From`, which performs the leading-byte sniff and routes to the right parser.

This means `FromURL` automatically handles any spec format the local importer supports — the network step is purely a fetch, never a format-specific decision. Users who already have a spec on disk skip the function and call `From` directly.
