# logging — structure

## Files

| File | Purpose |
| ---- | ------- |
| [logging.go](logging.go) | The package logger (`logger`, `L`), `Configure` / `SetOutput`, and the redaction helpers (`RedactHeaderValue`, `RedactURL`, the `sensitiveHeaders` set). |
| [logging_test.go](logging_test.go) | Redaction cases, the "no plaintext credential in a logged request line" acceptance test, and `Configure` file/no-file/error paths. |

## Types & symbols

| Symbol | Role |
| ------ | ---- |
| `logger` (package var) | The current `*slog.Logger`; defaults to a discard text handler so the package is a no-op until configured. |
| `L()` | Accessor for `logger`. |
| `Configure(verbose, logFile)` | Installs a text handler to stderr (+ an appended `0600` file when `logFile != ""`); returns a close func. |
| `SetOutput(w, verbose)` | Installs a text handler to `w` (test seam). |
| `RedactHeaderValue(name, value)` | `[redacted]` for sensitive header names, else the value. |
| `RedactURL(raw)` | Userinfo + query stripped; `[redacted url]` when unparseable. |
| `sensitiveHeaders` | Lower-cased set of header names treated as credentials. |
