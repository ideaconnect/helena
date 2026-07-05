# history — Structure

## Files

| File | Responsibility |
| --- | --- |
| [history.go](history.go) | The whole package: the `Entry` and `Store` types, `New` (load), `Record` (scrub + append + trim + persist), `Entries` (newest-first copy), `Len`, `Clear`, and the private `load` / `save` (atomic stage+rename, best-effort). |
| [history_test.go](history_test.go) | Newest-first ordering, the bounded ring (oldest dropped), persist+reload round-trip, over-cap trim on load, malformed-file tolerance, in-memory (blank path), scrub-on-record, and save-failure resilience. |

## Type catalog

| Type | Role |
| --- | --- |
| `Store` | Bounded, restart-persistent send history. Fields: `mu` (guards concurrent access), `path` (`""` disables persistence), `max` (entry cap), `entries` (oldest-first). |
| `Entry` | One recorded send: `Time`, `Method`, `URL`, `Status` (0 on a failed send), `Duration`, `Size` (response bytes), `Err`, and `Request` (the secret-scrubbed snapshot, enough to restore/resend). |
| `fileFormat` | The on-disk YAML shape (`entries: [...]`). |

## Constants

- `DefaultMax = 200` — the entry cap when `New` is given `max <= 0`.

## Invariants

- **No cleartext credentials on disk.** `Record` calls `storage.ScrubRequestSecrets`
  before storing, so `history.yml` mirrors the collection YAML's secret
  externalization (#42).
- **Best-effort persistence.** `save` / `load` swallow errors; history is a
  convenience, never load-bearing. A blank `path` keeps everything in memory
  (used by the headless runner and tests).
- **Newest-first for display, oldest-first in memory.** `entries` grows at the
  tail; `Entries()` reverses a copy so the UI shows the most recent send first.
  The copy is deep (`Request` is `Clone`d): Restore / Resend bind the returned
  request into the editor and edit it in place, which must not reach back and
  rewrite the stored entry.
