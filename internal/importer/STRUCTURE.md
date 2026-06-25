# importer — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level doc comment. |
| [from.go](from.go) | `From` dispatcher and the one-byte `looksLikeXML` sniffer. |
| [curl.go](curl.go) | `FromCurl` — parses a `curl` command line into a single `model.Request`; `tokenizeShell` (quote/escape/continuation-aware tokenizer), `splitFlag`, `bodyFromData` (Content-Type → body type, with JSON/form sniffing), `parseFormData`, `appendQuery`, `curlName`, and the `curlValueShorts`/`curlValueFlags` skip tables. |
| [curl_test.go](curl_test.go) | `FromCurl` + `tokenizeShell` tests: method/URL/headers/data variants, multipart, basic auth, `-G`, quoting, line continuations, noise-flag skipping, and error paths. |
| [openapi.go](openapi.go) | OpenAPI 3 / Swagger 2 parsing: `FromOpenAPI`, YAML→JSON normalization, OAS3-to-collection conversion. |
| [openapi_test.go](openapi_test.go) | OpenAPI tests with embedded `oas3Sample` / `swagger2Sample` fixtures (also reused by url_test.go and wsdl_test.go). |
| [postman.go](postman.go) | `FromPostman` + the `looksLikePostman` sniffer; the `pm*` decode structs (incl. `pmURL.UnmarshalJSON` accepting string-or-object URLs) and the body/auth mappers. |
| [postman_test.go](postman_test.go) | Postman tests: folder/request tree, header disabled-flag, query params, raw/urlencoded/formdata/graphql bodies, bearer/basic/apikey/noauth, URL reconstruction, detection, and `From` routing. |
| [wsdl.go](wsdl.go) | `FromWSDL`, the `wsdl*` XML structs, and the SOAP envelope template builder. |
| [wsdl_test.go](wsdl_test.go) | WSDL fixture (`wsdlSample`), `FromWSDL` tests, and the `From` dispatcher smoke test covering all three input flavors. |
| [url.go](url.go) | `FromURL`: HTTP fetch + dispatch through `From`. |
| [url_test.go](url_test.go) | URL-fetch tests for OpenAPI, WSDL, non-2xx and network failures. |

## Auto-detection: the `From` dispatcher

`From` ([from.go:7](from.go#L7)) is the public entry point. Postman is checked first (a structural JSON probe), then the one-byte XML rule:

```go
if looksLikePostman(data) {
    return FromPostman(data)
}
if looksLikeXML(data) {
    return FromWSDL(data)
}
return FromOpenAPI(data)
```

`looksLikePostman` ([postman.go](postman.go)) unmarshals a tiny probe and returns `true` only when the document has both an `info` object and an `item` array and carries neither an `openapi` nor a `swagger` key (so OpenAPI/Swagger never misroute). `looksLikeXML` ([from.go:18](from.go#L18)) skips leading whitespace (`' '`, `'\t'`, `'\n'`, `'\r'`) and returns `true` only if the next byte is `<`. Anything else — `{`, `o`, `s`, `#`, `-` (YAML), digits — falls through to `FromOpenAPI`, which then disambiguates Swagger 2 from OpenAPI 3 by inspecting the top-level `openapi` / `swagger` keys after YAML→JSON normalization.

This is deliberately simple: WSDL files always start with `<?xml` or `<definitions`, OpenAPI specs never do. There is no MIME sniff, no magic-bytes table.

## Type catalog

### Public parsers

- `From` — see above; dispatches on a single leading byte.
- `FromOpenAPI` ([openapi.go:20](openapi.go#L20)) — accepts JSON or YAML. Pipeline: `toJSON` -> probe `swagger`/`openapi` -> either `openapi2conv.ToV3` or `openapi3.NewLoader().LoadFromData` -> `convertOAS3`.
- `FromWSDL` ([wsdl.go:17](wsdl.go#L17)) — unmarshals into `wsdlDefinitions`, builds a binding-name lookup, then iterates services -> ports -> binding-operations, emitting one POST `model.Request` per operation. Errors if no operations are found.
- `FromURL` ([url.go:17](url.go#L17)) — `http.Get` with a settings-derived transport; non-2xx becomes an error mentioning `resp.Status`; the body is handed to `From`.

### OpenAPI helpers ([openapi.go](openapi.go))

- `toJSON` ([openapi.go:57](openapi.go#L57)) — fast-paths valid JSON; otherwise YAML-unmarshals and re-marshals as JSON. Exists because kin-openapi only consumes JSON.
- `normalizeYAML` ([openapi.go:72](openapi.go#L72)) — recursively converts `map[any]any` (still produced by `yaml.v3` for some nested maps) into `map[string]any`, which is what `json.Marshal` requires.
- `convertOAS3` ([openapi.go:94](openapi.go#L94)) — walks the OAS3 document: hoists `Servers[0].URL` into a `{{base_url}}` environment variable, groups paths-and-operations into folders by `Tags[0]`, leaves tag-less operations as root requests.
- `buildRequest` ([openapi.go:150](openapi.go#L150)) — assembles a single `model.Request`: prepends `{{base_url}}` when present, deduplicates path-item + operation parameters by `(in,name)`, maps `query` to `Params`, `header` to `Headers`, leaves `path` placeholders embedded in the URL.
- `bodyTypeFromContentType` ([openapi.go:226](openapi.go#L226)) — case-insensitive content-type sniff: `json`, `xml`, `form-urlencoded`, `multipart`, `text/*`, default text.
- `extractExample` ([openapi.go:244](openapi.go#L244)) — pulls `Example`, then any first `Examples[i].Value.Value`, then `Schema.Value.Example`.
- `formatExample` ([openapi.go:259](openapi.go#L259)) — strings pass through; structured values are pretty-printed JSON.
- `sortedKeys` / `sortStrings` ([openapi.go:277](openapi.go#L277), [openapi.go:288](openapi.go#L288)) — keep import output deterministic without adding a `sort` import.

### Postman parser ([postman.go](postman.go))

- `FromPostman` ([postman.go:14](postman.go#L14)) — unmarshals the v2.x document and walks `item` recursively via `pmAppendItem` (request leaf vs. folder), mapping collection/folder/request auth through `pmConvertAuth`.
- `pmConvertRequest` / `pmConvertBody` / `pmConvertAuth` — field mappers from the `pm*` decode structs to `model.Request` / `Body` / `Auth`. `pmConvertBody` switches on `mode` (raw/urlencoded/formdata/graphql); `pmRawBodyType` reads `options.raw.language`.
- `pmURL.UnmarshalJSON` ([postman.go:264](postman.go#L264)) — accepts a URL as a bare string or an object; `effectiveRaw` reconstructs from host + path when `raw` is absent. `pmStringList` decodes host/path as either a string array or a single string.
- `looksLikePostman` — structural sniffer; see "Auto-detection" above.

### WSDL types and helpers ([wsdl.go](wsdl.go))

The `wsdl*` structs capture only the fields Helena needs; everything else is ignored by the XML decoder.

- `wsdlDefinitions` ([wsdl.go:118](wsdl.go#L118)) — root `<definitions>` element with `targetNamespace`, services, bindings.
- `wsdlService` ([wsdl.go:127](wsdl.go#L127)) — `name` attr (used as collection name) + ports.
- `wsdlPort` ([wsdl.go:133](wsdl.go#L133)) — binding reference + `<address>` element holding the endpoint URL.
- `wsdlSoapAddress` ([wsdl.go:140](wsdl.go#L140)) — just `location` (the SOAP endpoint URL).
- `wsdlBinding` ([wsdl.go:145](wsdl.go#L145)) — binding name plus its operations.
- `wsdlBindingOperation` ([wsdl.go:152](wsdl.go#L152)) — operation name + nested `<soap:operation>`.
- `wsdlSoapOperation` ([wsdl.go:158](wsdl.go#L158)) — `soapAction`; empty means no `SOAPAction` header is emitted.

Helper functions:

- `firstServiceName` ([wsdl.go:60](wsdl.go#L60)) — picks the collection name; falls back to a generic label when no services are declared.
- `localXMLName` ([wsdl.go:68](wsdl.go#L68)) — strips a namespace prefix (`tns:CalcBinding` -> `CalcBinding`) so port-binding references match binding names.
- `buildSOAPRequest` ([wsdl.go:75](wsdl.go#L75)) — emits the placeholder SOAP 1.1 envelope, content-type header, and SOAPAction header (when present).

### Detection sniffer ([from.go](from.go))

- `looksLikeXML` — see "Auto-detection" above.
