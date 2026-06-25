# assertion — Workflow

## Evaluating a request's assertions after Send
1. After the leaf request returns, the UI's `chainExecutor.ExecuteOnce` calls `assertion.Evaluate(req.Assertions, resp.StatusCode, resp.Headers, resp.Body)`.
2. `Evaluate` skips disabled rows and calls `evalOne` for each enabled one.
3. `evalOne` resolves the row's `Source` via `extract` into `(actual, found)`:
   - `res.status` → the status code; `res.body` → the raw body; `res.header.<Name>` → the first header value; `res.json.<path>` → a `jsonPath` walk.
4. For `exists` / `notExists` the result is just `found`. Otherwise a missing value fails immediately; a found value is compared by operator:
   - `equals` / `notEquals` / `contains` / `notContains` — string comparison.
   - `matches` — `regexp.Compile(Expected)` then `MatchString` (a bad pattern fails the row).
   - `greaterThan` / `lessThan` — both sides parsed as `float64` (a non-numeric side fails the row).
5. Each row becomes a `Result{Name, Passed, Error}`; the `Name` is the row rendered as `"<source> <op> <expected>"`.

## Surfacing results
The UI converts each `assertion.Result` into a `scripting.TestResult` and merges
it with the script `test()`/`expect()` results, so declarative assertions and
scripted assertions render together as `PASS` / `FAIL` lines plus a summary in
the Scripts console (`formatTestResults` in [internal/ui/execution.go](../ui/execution.go)).

## Round-trip
`model.Assertion` rows persist per request via the `assertions:` list in the
request YAML (`ocAssertion` in [internal/storage/opencollection.go](../storage/opencollection.go));
the enabled flag is stored inverted as `disabled` to match the param/header
convention.
