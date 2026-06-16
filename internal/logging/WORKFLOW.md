# logging — workflows

## Startup

1. `cmd/helena` parses `--verbose` and `--log-file` (falling back to the
   `HELENA_LOG` env var for the file).
2. It calls `logging.Configure(verbose, logFile)`. Without a file, logs go to
   stderr only; with one, an `io.MultiWriter(os.Stderr, file)` tees them and the
   returned close func is `defer`-ed.
3. `Configure` builds a `slog.TextHandler` at `LevelDebug` (verbose) or
   `LevelInfo` and stores it in the package `logger`. Until this runs, `L()`
   returns a discard logger, so any earlier `L().Info(...)` is silently dropped.

## What gets logged (and how it stays safe)

- **Startup** — `helena starting` with version/commit.
- **Each Send** — `internal/ui`'s `send` logs method + `RedactURL(req.URL)` +
  request name. The URL is **always** passed through `RedactURL` so a
  `user:pass@host` or `{{token}}`-expanded query never reaches the log.
- **Recovered panics** — the process-level recover in `main` and the UI
  `guard` both log the recovered value plus `debug.Stack()` at `Error` level,
  so a crash leaves an actionable breadcrumb.

## Redaction contract

Credentials must never be logged verbatim:

- Header values go through `RedactHeaderValue`, which returns `[redacted]` for
  `Authorization` / `Proxy-Authorization` / `Cookie` / `Set-Cookie` /
  `X-API-Key` (case-insensitive) and the original value otherwise.
- URLs go through `RedactURL`, which masks userinfo (`user:pass@` →
  `redacted@`) and replaces the query string with `redacted`, keeping the host
  and path for diagnosis.

This mirrors the error-surface redaction in `internal/auth` /
`internal/httpclient` (#112) so logs and UI errors apply the same rule.
