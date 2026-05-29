# importer — Workflow

## Auto-detecting OpenAPI vs WSDL

`From` ([from.go:7](from.go#L7)) makes one decision based on a single byte:

1. Skip leading whitespace (`' '`, `'\t'`, `'\n'`, `'\r'`).
2. If the next byte is `<`, the input is treated as XML and forwarded to `FromWSDL`.
3. Otherwise — JSON `{`, YAML `o`/`s`/`#`/`-`, or anything else — it goes to `FromOpenAPI`, which then disambiguates Swagger 2 vs OpenAPI 3 by probing for `swagger` / `openapi` keys.

WSDL files always start with `<?xml` or `<definitions>`; OpenAPI documents never do. The sniff is intentionally minimal — there is no MIME table, no content-type check, no full parse-and-retry.

## Hoisting OpenAPI server URL to `{{base_url}}`

`convertOAS3` ([openapi.go:94](openapi.go#L94)) inspects `doc.Servers`. If at least one server is declared, the first server's `URL` is hoisted into a single-environment, single-variable structure:

```text
Collection.Environments = [Environment{
    Name: "Default",
    Variables: [Variable{Enabled: true, Key: "base_url", Value: doc.Servers[0].URL}],
}]
```

`buildRequest` ([openapi.go:150](openapi.go#L150)) then uses `hasBaseVar=true` to prefix every request's URL with `{{base_url}}`. When `doc.Servers` is empty, no environment is created and the request URL is the raw path. This means edits to the environment update every imported request consistently — which is the point of hoisting.

## Mapping OpenAPI `requestBody.Content` example to `model.Body`

For each operation, `buildRequest` iterates `op.RequestBody.Value.Content` in sorted key order and picks the **first** media-type entry it finds (`break` after the first non-nil one):

1. Content-type string -> `model.BodyType` via `bodyTypeFromContentType` ([openapi.go:226](openapi.go#L226)) — substring matches for `json`, `xml`, `form-urlencoded`, `multipart`, `text/`, defaulting to `BodyText`.
2. Example body string -> `extractExample` ([openapi.go:244](openapi.go#L244)) which prefers `mt.Example`, then any `mt.Examples[*].Value.Value`, then `mt.Schema.Value.Example`.
3. Structured examples are pretty-printed JSON via `formatExample` ([openapi.go:259](openapi.go#L259)); string examples pass through verbatim.

Sorting the media-type keys gives deterministic output — without it, the choice between e.g. `application/json` and `application/xml` would depend on map iteration order.

Parameters are handled separately from bodies: `query` params -> `r.Params`, `header` params -> `r.Headers`, `path` params remain embedded in the URL as `{name}` placeholders. Optional params are imported disabled, required params are imported enabled.

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
