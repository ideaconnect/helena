# logging

Helena's diagnostic logging facility: a process-wide `slog.Logger` plus
redaction helpers, so a log file can capture send round-trips and recovered
panics for field bug reports **without ever leaking credentials**.

It is a no-op (discards everything) until `Configure` runs, so importing it is
always safe and tests stay quiet by default.

## Public API

- `L() *slog.Logger` — the process logger (discards until configured).
- `Configure(verbose bool, logFile string) (func() error, error)` — install the
  logger. Always writes to stderr; when `logFile` is non-empty also appends to
  it (mode `0600`), returning a close func. `verbose` raises the level from
  Info to Debug. Returns a safe (non-nil) close func even on error.
- `SetOutput(w io.Writer, verbose bool)` — point the logger at an arbitrary
  writer (used by tests).
- `RedactHeaderValue(name, value string) string` — masks the value of a
  credential header (Authorization, Proxy-Authorization, Cookie, Set-Cookie,
  X-API-Key); other headers pass through.
- `RedactURL(raw string) string` — strips userinfo and the query string,
  keeping scheme/host/path; fully redacts an unparseable input.

## Wiring

`cmd/helena` parses `--verbose` / `--log-file` (or the `HELENA_LOG` env),
calls `Configure`, logs startup, and logs a fatal panic with `debug.Stack()`.
`internal/ui` logs each Send (with `RedactURL`) and logs recovered callback
panics (with the stack) through `guard`.

## Dependencies

Standard library only (`log/slog`, `net/url`, `io`, `os`, `strings`).
