# internal/pathparam — workflows

## Listing a request's path parameters (editor)

The **Path** tab derives its rows from the URL, live:

1. On load, and on every URL-field edit, the UI calls `Names(currentRequest.URL)`.
2. Each returned name becomes a row; its value is pulled from
   `Request.PathParams` by name (empty when the user hasn't filled it).
3. Typing a value upserts `{Key: name, Value: v}` into `Request.PathParams`.
   The tab never mutates `PathParams` merely by being shown, so opening then
   saving an untouched request stays byte-identical on disk.

## Filling path parameters (send / export)

Inside `httpclient.Build`, after `{{variable}}` resolution produces the raw
URL:

1. `Apply(rawURL, lookup)` walks the resolved URL.
2. `lookup(name)` scans `Request.PathParams` for an enabled entry whose
   `Key == name`; it resolves that entry's `Value` (so a path value may itself
   reference `{{variables}}` and `{{?prompts}}`) and returns it when non-empty.
3. An unmatched or empty token is left as literal `{name}` — it reaches the
   wire unchanged and is flagged in the URL preview, so a forgotten parameter
   is visible rather than silently dropped.

Because `Apply` never re-scans inserted values, a resolved value containing
braces cannot trigger further substitution.
