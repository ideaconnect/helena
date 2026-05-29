# responsefmt — Workflow

## Pretty-printing a JSON response
1. `httpclient` returns the raw body bytes plus `Content-Type` from the response headers.
2. UI calls `responsefmt.IsJSON(contentType)`; if true, it tries pretty-printing.
3. UI calls `PrettyJSON(body)`; `json.Indent` re-emits the document with two-space indentation.
4. On success the pretty string is shown in the Body tab; on error the UI falls back to the raw bytes (the JSON was malformed).

## Pretty-printing an XML/SOAP response
1. UI calls `IsXML(contentType)` (matches `application/xml`, `text/xml`, `application/soap+xml`).
2. UI calls `PrettyXML(body)`; the function decodes tokens, drops whitespace-only `CharData`, then re-encodes with two-space indent so existing indentation doesn't compound.
3. Malformed XML returns an error; the UI shows the raw body instead.

## Rendering the response headers panel
1. UI hands `resp.Header` (an `http.Header`) to `FormatHeaders`.
2. The function sorts header names alphabetically and walks each name's value slice, writing `Key: value\n` per value.
3. The resulting string drops into a read-only text view in the UI.

## Showing response stats
1. After a request finishes, UI knows `len(body)` and the wall-clock duration of the call.
2. `HumanSize(int64(len(body)))` returns a compact string like `1.5 KB` or `2.0 MB`.
3. `HumanDuration(elapsed)` returns `250 ms`, `1.20 s`, or `1m 5s`.
4. Both strings appear in the response footer next to the status code.
