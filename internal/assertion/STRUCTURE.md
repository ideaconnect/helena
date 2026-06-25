# assertion — Structure

## Files

| File | Responsibility |
| --- | --- |
| [assertion.go](assertion.go) | The operator constants + `Operators` list, the `Result` type, the `Evaluate` entry point, the per-row `evalOne`, the `extract` source resolver, and the `jsonPath` / `scalarString` JSON-body walkers. |
| [assertion_test.go](assertion_test.go) | Every operator against a representative response, disabled-row skipping, unknown source/operator handling, and the JSON-path edge cases (nested object, null, out-of-range, non-container segment, non-JSON body). |

## Type catalog

| Type | Role |
| --- | --- |
| `Result` | One assertion outcome: `Name` (the row rendered as a label), `Passed`, and `Error` (the failure message). Field-compatible with `scripting.TestResult` so the UI formats both as `PASS`/`FAIL` console lines. |

The operator set is exported as untyped string constants (`OpEquals`, …) plus
the `Operators` slice for the UI picker; assertions store the operator as a
plain string in `model.Assertion.Op`.

## Non-trivial internals

### `extract` source resolution — [assertion.go:125](assertion.go#L125)
A small prefix switch maps `res.status` / `res.body` / `res.header.<Name>` /
`res.json.<path>` to a `(value, found)` pair. An unrecognized source is simply
"not found", which fails a comparison (or satisfies `notExists`).

### `jsonPath` — [assertion.go:146](assertion.go#L146)
Unmarshals the body to `any` and walks a dotted path; a numeric segment indexes
a JSON array, a string segment keys an object. Any missing key, out-of-range
index, or attempt to descend into a scalar returns `("", false)`.
