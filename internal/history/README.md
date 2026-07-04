# internal/history

A bounded, restart-persistent log of sent requests and their response summaries
(#65) — the data behind the GUI's **Help → History** viewer, so a user can
revisit, restore, or resend a past request.

## Public API

| Symbol | Purpose |
| --- | --- |
| `Store` | The history store: an in-memory ring mirrored to a YAML file. Safe for concurrent use. |
| `New(path string, max int) *Store` | Open a store backed by `path` (loads any existing history). A blank `path` keeps history in memory only; `max <= 0` uses `DefaultMax` (200). |
| `(*Store).Record(Entry)` | Append a send: the request is secret-scrubbed, a zero `Time` is stamped now, the ring is trimmed to the cap, and the file is rewritten (best-effort). |
| `(*Store).Entries() []Entry` | A newest-first copy of the recorded entries. |
| `(*Store).Len() int` | Number of recorded entries. |
| `(*Store).Clear()` | Empty the history and rewrite the file. |
| `Entry` | One recorded send: `Time`, `Method`, `URL`, `Status`, `Duration`, `Size`, `Err`, and the (scrubbed) `Request`. |

## Privacy

Every recorded `Request` is run through `storage.ScrubRequestSecrets` before it
is stored, so `history.yml` **never carries a cleartext credential** — the same
invariant the collection YAML has via secret externalization (#42). A restored
request therefore keeps everything but the secret, which a resend re-resolves
from the active environment.

## Persistence

The store writes `history.yml` (next to `config.yml`, under the OS config dir)
atomically — stage to a temp file, then rename — so a crash mid-write can't
truncate it. Persistence is **best-effort**: a write failure is swallowed (the
history is a convenience, not load-bearing state) and a missing / malformed file
loads as an empty history.

## Dependencies

- [internal/model](../model) — `model.Request` (the stored snapshot).
- [internal/storage](../storage) — `ScrubRequestSecrets` (the secret scrub).
- `gopkg.in/yaml.v3` — the on-disk format.

The GUI wires a `Store` onto the `Session` (`session.New` → `Session.History()`)
and records from the send path; the headless runner does not record.
