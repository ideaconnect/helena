# responsefmt — Workflow

## Validating / formatting a request body
1. In the request editor's Body tab the user picks JSON or XML and clicks Validate or Format.
2. UI calls `PrettyJSON(body)` / `PrettyXML(body)`. `PrettyJSON` runs `json.Indent`; `PrettyXML` decodes tokens, drops whitespace-only `CharData`, then re-encodes with a two-space indent so existing indentation doesn't compound.
3. An error means the body is malformed (surfaced as "JSON/XML invalid"); on success Format replaces the editor text with the re-indented output.

> The response Body is no longer rendered here — it goes to the external
> `go-fyne-pretty-view` widget, which parses, highlights and format-detects on
> its own. The former response pretty-print / highlight / structured-tree flows
> are gone.

## Rendering the response headers panel
1. UI hands `resp.Header` (an `http.Header`) to `FormatHeaders`.
2. The function sorts header names alphabetically and walks each name's value slice, writing `Key: value\n` per value.
3. The resulting string drops into the read-only Headers tab.

## Showing response stats
1. After a request finishes, UI knows `len(body)` and the wall-clock duration of the call.
2. `HumanSize(int64(len(body)))` returns a compact string like `1.5 KB` or `2.0 MB`.
3. `HumanDuration(elapsed)` returns `250 ms`, `1.20 s`, or `1m 5s`.
4. Both strings appear in the response status line next to the status code.
