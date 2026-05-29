# model — Workflow

`model` has no runtime of its own; it is a passive set of types. The flows below describe how the rest of Helena uses these types end-to-end.

## Creating a new request in the tree
1. UI calls `model.NewID()` to mint a fresh hex ID.
2. UI builds a `Request{ID: id, Name: "Untitled", Method: GET, Body: Body{Type: BodyNone}}`.
3. The request is appended to a `Folder.Requests` or `Collection.Requests` slice.
4. `storage.Save` walks the `Collection` and writes each `Request` to its OpenCollection YAML file.

## Sending a request
1. UI hands the active `Request` to `httpclient.Do`.
2. `httpclient` calls `model.EnabledPairs(req.Headers)` and `EnabledPairs(req.Params)` to drop disabled rows.
3. `req.Method` is checked with `Method.Valid()` (defensive — UI only offers valid methods).
4. If `req.Body.Type` is JSON/XML/text/form, the implied header comes from `BodyType.ContentType()`; multipart sets its own boundary.
5. The transport, redirects, and timeout are governed by the active `Settings` values (`InsecureSkipVerify`, `FollowRedirects`, `TimeoutSeconds`).

## Resolving variables against an environment
1. The session collects `Variable` entries from the active `Environment`, dropping any with `Enabled == false`.
2. The `(Key, Value)` pairs become a scope map handed to `vars.New`.
3. `vars.Resolve` substitutes `{{name}}` references inside `Request.URL`, headers, params, and `Body.Content`.
4. `Variable.Secret == true` does not affect substitution; it only tells the UI to mask the field.

## Loading/saving the app config
1. `config.Load` reads `config.yml`, populating `Config.Settings` from `model.Settings`.
2. If the file is missing, `config.Default()` falls back to `model.DefaultSettings()`.
3. On save, `model.Settings` is marshaled into the same YAML file alongside workspace metadata.
