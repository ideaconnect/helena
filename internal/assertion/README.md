# assertion

`assertion` evaluates declarative (no-code) response checks (#88). A request
carries a list of `model.Assertion` rows — `{Source, Op, Expected, Enabled}` —
which `Evaluate` runs against a Send's response, producing pass/fail `Result`s
that share the `test()`/`expect()` results surface (rendered as `PASS`/`FAIL`
lines in the Scripts console).

The package is pure and stateless: it takes the status code, headers, and body
and returns one `Result` per enabled assertion. It never errors — a malformed
source, bad regex, or non-numeric comparison becomes a failing `Result`.

## Public API

- `Evaluate(assertions []model.Assertion, status int, headers http.Header, body []byte) []Result` — run every enabled assertion, returning one `Result` each (disabled rows skipped).
- `Result{Name string; Passed bool; Error string}` — one outcome, shaped like `scripting.TestResult` so the UI renders both through one formatter.
- `Operators []string` and the `Op*` constants — the supported operator set for the UI picker: `equals`, `notEquals`, `contains`, `notContains`, `matches` (regex), `greaterThan`, `lessThan`, `exists`, `notExists`.

### Source expressions

| Source | Resolves to |
| --- | --- |
| `res.status` | the numeric status code |
| `res.body` | the raw response body |
| `res.header.<Name>` | a response header value (first) |
| `res.json.<dotted.path>` | a value inside a JSON body; numeric path segments index arrays (e.g. `res.json.items.0.id`) |

## Dependencies

- `encoding/json`, `net/http`, `regexp`, `strconv`, `strings`, `fmt` — standard library.
- [`github.com/idct/helena/internal/model`](../model) — `Assertion`.
