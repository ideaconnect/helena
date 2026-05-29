# exporter

`exporter` renders a `model.Request` as a runnable shell command for another tool — `curl` and `wget` today, more languages later. The rendered command is intended to be copy-pasteable into a terminal and to reproduce exactly what Helena itself would send.

The package is a thin layer over [`httpclient.Build`](../httpclient/httpclient.go). It does not rebuild the wire request from `model.Request` directly; instead it calls `httpclient.Build`, then walks the resulting `*http.Request` to emit the command flags. This is a deliberate **fidelity guarantee**: the same variable resolution, the same enabled-row filtering, the same auto-Content-Type behavior, the same URL parsing. If `Do` would send it, the exporter prints it.

Shell quoting is handled in-package via three small helpers (`shellQuote`, `alwaysQuote`, `needsShellQuote`) — no external dependency on `shlex` or `shellescape` is needed.

## Public API

- `ToCurl(r model.Request, res *vars.Resolver, s model.Settings) (string, error)` — renders a multi-line `curl` command honoring `Settings` flags (insecure TLS, follow redirects, timeout).
- `ToWget(r model.Request, res *vars.Resolver, s model.Settings) (string, error)` — same semantics as `ToCurl`, but emits a `wget` command (note: wget's redirect semantics are inverted vs curl, so the flag set differs).

## Dependencies

- [`github.com/idct/helena/internal/httpclient`](../httpclient) — `httpclient.Build` for the wire request; reused for byte-equivalent fidelity.
- [`github.com/idct/helena/internal/model`](../model) — `Request`, `Settings`.
- [`github.com/idct/helena/internal/vars`](../vars) — `*vars.Resolver` for `{{var}}` substitution.
- `context`, `fmt`, `io`, `net/http`, `sort`, `strings` — standard library.
