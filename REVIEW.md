# Helena — Deep Code Review & Remediation Plan

_Produced by a multi-agent review: 22 reviewers (package-scoped + cross-cutting
lenses) → adversarial verification of every finding → categorisation. 100 raw
findings, **97 survived verification** (3 dropped as false positives), 176 agents.
The synthesis/plan agents were cut off by a session token limit, so this plan was
assembled by hand from the 97 verified findings. Reviewed against the current
on-disk (uncommitted) state._

## Severity / category

| | bug | concurrency | security | error-handling | performance | testing | code-quality | architecture | docs |
|---|---|---|---|---|---|---|---|---|---|
| **high** (8) | 5 | 2 | 1 | | | | | | |
| **medium** (12) | 5 | 1 | 1 | 1 | 1 | 1 | 1 | | 1 |
| **low** (77) | 18 | 2 | 7 | 13 | 10 | 9 | 10 | 2 | 6 |

No critical-severity issues.

## Executive assessment

Helena is a well-structured, well-tested codebase (~12k LOC code / ~12.7k LOC
test, per-module docs, explicit invariants in `AGENTS.md`). It honours most of
its stated invariants and the review found **no critical issues**. Risk is
concentrated in five areas:

1. **A few real correctness/safety bugs** — `config.Load` doesn't apply
   `DefaultSettings` (a config without a `settings:` block silently runs with an
   *unlimited timeout*, redirects off, CORS advisory off); the OpenAPI importer
   panics (crashing the app) on a spec without an `info` block; tree mutators
   persist the *active* collection rather than the one they changed;
   `RemoveCollection` can delete the wrong workspace entry (data loss).
2. **Send-path data races** — the off-UI-goroutine isolation is incomplete: the
   leaf request's `Chain`/`Params`/`Headers`/`Body.Form` slices stay aliased with
   the live `m.currentRequest` and are read on the worker while the UI thread can
   still edit them. `go test -race` (mandated by `AGENTS.md`) would flag this.
3. **Resource-exhaustion foot-guns** — response bodies are read with unbounded
   `io.ReadAll`, and the script timeout doesn't preempt native built-ins, so a
   single line of an imported (executable) collection can OOM/freeze the app.
4. **Fidelity gaps** — `PrettyXML` corrupts namespaced/SOAP XML on Format;
   cURL/wget export drops a custom `Host`; the path-param `type` discriminator is
   lost on round-trip; `writeYAML` is non-atomic (corruption on crash mid-save).
5. **Accumulated doc drift + coverage gaps** — several `STRUCTURE.md`/`WORKFLOW.md`
   files describe removed/renamed symbols and stale signatures; the `session`
   package has dipped below the 90% floor on the move/cascade paths.

The dominant structural issue is **`internal/ui/shell.go` at 1417 lines** acting
as a god-object. Most high findings are small, surgical fixes.

---

# Remediation plan

Each behaviour-changing task must ship with a paired test and pass
`go test ./... -race` (per `CLAUDE.md`). Effort: S ≈ <1h, M ≈ a few hours, L ≈ a day+.

## P0 — Safety, data-loss & races (do first; mostly small, surgical)

**Status: ✅ all 7 done — each with a paired test; `gofmt`/`go vet` clean and `go test ./... -race` green.**

- [x] **Config: apply `DefaultSettings` on load.** `internal/config/config.go:88-99`
  seeds only `Workspaces`/`Active`; `Settings` stays zero (`TimeoutSeconds=0` =
  unlimited, `FollowRedirects=false`, `CORSWarning=false`) for any file lacking a
  `settings:` block. Seed `c := Default()` before `yaml.Unmarshal` (let the file
  overwrite present keys), then re-default `Workspaces` if empty. **Test:** load
  YAML with no `settings:` → expect `DefaultSettings`. _(S, HIGH)_
- [x] **Send-path slice aliasing → deep-copy on the UI thread.** In `send()` right
  after `req = *m.currentRequest` (`internal/ui/shell.go:1047`) clone the
  slice-backed fields before the `go func()`: `req.Chain`, `req.Params`,
  `req.Headers`, `req.Body.Form` (or a snapshot helper mirroring
  `session.deepCopyRequest`). Closes the HIGH chain-race, HIGH send-snapshot-race,
  and MED params/headers race in one change. Fix the misleading comment at
  `shell.go:1057` (`ExecuteOnce` does **not** copy `Chain`). **Test:** `-race` test
  that starts a send and concurrently mutates `currentRequest.Chain`/`Params`. _(S, HIGH×2 + MED)_
- [x] **OpenAPI importer nil-`Info` panic.** `internal/importer/openapi.go:96`
  dereferences `doc.Info.Title`; a valid spec without `info` panics and crashes
  the app (the UI callbacks `internal/ui/import.go:85,102` have no `recover`).
  Guard the deref (`name := "Imported API"; if doc.Info != nil { … }`); ideally
  return an error rather than panic. **Test:** `{"openapi":"3.0.0","paths":{}}`. _(S, HIGH)_
- [x] **`RemoveCollection` deletes the wrong entry / wrong guard.**
  `internal/session/session.go:168-184` removes by raw index and mis-pairs when an
  earlier collection failed to load (data loss surviving restart); the
  `:169-171` guard tests `activeCol` instead of workspace validity (blocks valid
  removals). Resolve removal by `dir` (mirror `MoveCollection`'s misalignment
  guard) and replace the guard with `Active`-range validity. **Test:** remove with
  a preceding unloadable collection. _(S/M, HIGH + MED)_
- [x] **Tree mutators persist the active collection, not the mutated one.**
  Add/Rename/Delete/Duplicate call `SaveActiveCollection()` (`internal/session/items.go:30,60,81,134,259,280,293`).
  Switch to `saveCollection(nodeCollectionIndex(id))` (helpers already exist).
  Latent today (masked by tree `OnSelected`) but fragile. _(M, HIGH)_
- [x] **Unbounded response body → OOM.** `internal/httpclient/httpclient.go:194`
  `io.ReadAll(resp.Body)` with no cap. Wrap in `io.LimitReader` with a
  configurable max (default ~50–100 MB) and set a truncation flag on `Response`
  (like `CORSWarning`). Apply the same cap to `internal/importer/url.go:33`.
  Closes MED security + MED script-runtime perf + LOW FromURL. **Test:** oversized
  body is truncated, not fully buffered. _(M, HIGH-adjacent)_
- [x] **Script timeout doesn't preempt native built-ins.**
  `internal/scripting/bindings.go:109-150` — a single builtin call (e.g. huge
  alloc) bypasses the 5s `Interrupt` and can OOM/freeze. Run `vm.RunString` in its
  own goroutine; on timeout/ctx-cancel, `Interrupt` **and** abandon the goroutine
  (return the timeout to the caller without waiting). Combine with the body cap
  above. **Test:** a script that allocates past the cap times out. _(M, HIGH security)_

## P1 — Fidelity bugs & performance

**Status: 7/13 done — P1-A (PrettyXML, atomic writeYAML, Duplicate-Chain, Host export, folder-drop-onto-root, chain RequestID) + the OAuth security cluster, each tested, `-race` green. Remaining: HTTP-client reuse, per-keystroke Query rebuild, cross-host header strip, vars, path-param type, and the minor-correctness batch.**

- [x] **`PrettyXML` corrupts namespaced/SOAP XML on Format.**
  `internal/responsefmt/responsefmt.go:30-54` round-trips through `encoding/xml`,
  mangling `xmlns:` prefixes — including SOAP envelopes from this repo's own WSDL
  importer. Minimum: detect `xmlns` and refuse (return error) so Format never
  silently corrupts; better: re-indent the raw token stream. **Tests:** namespaced + CDATA. _(M, HIGH)_
- [x] **`writeYAML` is non-atomic.** `internal/storage/store.go:484-491` — a crash
  mid-write truncates a collection file (fatal for `opencollection.yml`). Write to
  a temp file in the same dir + `os.Rename`. _(M, MED)_
- [x] **`DuplicateItem` aliases the `Chain` backing array.**
  `internal/session/items.go:547-559` `deepCopyRequest` clones other slices but not
  `Chain`; add `r.Chain = slices.Clone(r.Chain)`. **Test:** duplicate + mutate
  original's `Chain[0]`, assert copy unchanged. _(S, MED)_
- [x] **cURL/wget export drops a custom `Host` header.**
  `internal/exporter/exporter.go:54-58,82-86` — emit `-H 'Host: …'` when
  `req.Host` differs from the URL host. **Test.** _(S, MED)_
- [ ] **Path-param `type` lost on round-trip.** `internal/storage/opencollection.go:169`
  → `store.go:230` `mergeParamExtras` doesn't carry `Type`, so Bruno path params
  become query params on first save. Carry `Type` in the merge. _(M, MED)_
- [x] **Folder drop onto a collection-root's top half fails.**
  `internal/ui/treedrag.go:121-124` yields `dstParent=""` → misleading
  "cross-collection" error. Route to root `id`, index 0. **Test.** _(S, MED)_
- [x] **Chain `RequestID` re-derived on every rebuilt row.**
  `internal/ui/chain.go:78-113` — wrap `rebuildChainRows` in the `m.loading` guard
  so seeding values doesn't drop an ID pin (invariant 14). _(S, MED)_
- [ ] **HTTP client rebuilt per Send.** `internal/httpclient/httpclient.go:60-73`
  + `internal/ui/shell.go:1061-1068` construct a fresh `Client`/`Transport` (and
  OAuth2 resolver) every send — no connection reuse, idle conns accumulate. Reuse
  a per-session client. _(M, LOW-perf, high value)_
- [ ] **`applyURLEdit` rebuilds the whole Query container per keystroke.**
  `internal/ui/shell.go:864-869` — update rows in place / debounce. _(M, LOW-perf, user-visible)_
- [x] **OAuth correctness & security cluster** (done: refresh-token reuse, per-key serialization, complete CacheKey, default token-endpoint timeout, expires_in honoured) (`internal/auth/oauth2*.go`):
  use `refresh_token` before re-running the grant (today an expired auth-code
  token re-opens the browser); make `Token()` check-then-fetch atomic per key
  (avoids duplicate browser tabs / listeners); include `ClientSecret`/`RedirectURI`/
  `UsePKCE` in `CacheKey`; give the client-credentials token POST a timeout (not
  `http.DefaultClient`). _(M, several LOW security/concurrency)_
- [ ] **Strip auth headers on cross-host redirect.**
  `internal/httpclient/httpclient.go:126-138` — API-key-in-header survives a
  redirect to another host (credential leak). _(S, LOW security)_
- [ ] **Vars: stop re-expanding substituted values** (injection) and fix the
  deeper-than-`maxPasses` mis-report (`internal/vars/vars.go:53-76`). _(M, LOW)_
- [ ] Minor correctness: `corsAdvisory` credentials case (`httpclient.go:251`);
  API-key query re-encode reorders/drops params (`auth.go:130`); query merge
  reorders pre-existing URL params (`httpclient.go:126`); URL fragment absorbed
  into last query value (`query.go:18`); OAuth default `expires_in` applied on a
  bad value (`oauth2.go:241`). _(S each, LOW)_

## P2 — Architecture & code quality

- [ ] **Decompose `shell.go` (1417 lines).** Extract: the send/chain executor →
  `send.go`; the KV-row machinery + `rebuild*` → `kvrows.go` (Query/Content-Type
  sync already in `query.go`); the env/settings/workspace dialogs → `dialogs.go`.
  Leave `shell.go` as layout + wiring. Pure move-refactor gated by existing UI
  tests + build; do **after** P0/P1 land. _(L, architecture)_
- [ ] **Collapse the 5 near-identical pass-through sub-theme structs**
  (`internal/ui/helena_theme.go:121-229`) onto one embedded base. _(S, code-quality)_
- [ ] Make export snippet entries read-only (`internal/ui/export.go`); fix
  console JSON fallback printing Go pointers (`bindings.go:86-102`); reject unknown
  leaf kinds in `parseLeaf` (`items.go:535`); stop `SetActiveEnv` polluting the env
  map with a `-1` key (`session.go:255`). _(S each)_
- [ ] **Feature gap:** form-urlencoded/multipart body types have no Form-fields
  editor (`internal/ui/shell.go:307-332`). _(M)_

## P3 — Testing & docs

- [ ] **Restore `session` to ≥90% coverage** — move/cascade, `clampIndex`
  boundaries, `DuplicateItem` error paths (`internal/session/items.go:309-461`). _(M, MED)_
- [ ] **Paired regression tests for every P0/P1 fix** (CLAUDE.md mandate):
  importer malformed input, exporter `Host`, storage slug-collision + path-type,
  `helena_theme` sub-themes/`themedIcon`, the send() `-race` window, vars edge cases. _(M)_
- [ ] **Doc-drift sweep (batch):** `cmd/helena` tooltip layer
  (STRUCTURE/WORKFLOW); chain docs reference a non-existent `sessionRequestFinder`
  + stale `Resolve` signature; exporter WORKFLOW `Build` signature (3→4 args);
  scripting WORKFLOW ("chains not yet shipped"); auth `ErrOAuth2NotImplemented`
  doc; `treedrag` "cached plan" comment; `tabstrip` "fixed-width" claim. _(S, batch)_

## Ordering & risk notes

- The **slice deep-copy (P0)** and **`LimitReader` (P0)** are load-bearing,
  low-risk, and each closes multiple findings — do them first.
- The **config defaults (P0)** changes load behaviour; write the test first and
  re-check existing `config` tests.
- The **persist-by-collection (P0)** fix is latent (masked by tree selection);
  safe, but verify against `shell.go` `OnSelected`.
- The **OAuth cluster (P1)** is security-sensitive; land `CacheKey` + redirect
  header-strip with tests.
- The **`shell.go` decomposition (P2)** is large; do it last so you're not
  refactoring around unfixed bugs — it's a pure move gated by the UI test suite.

---

# Appendix — all 97 verified findings

_Sorted by severity then category. `~` = verifier marked 'needs-nuance' (real but rescoped). Locations are file:line as cited; confirm before editing._

## HIGH (8)

### [bug] Partial config (no `settings:` block) silently loads unsafe zero-value Settings
- **Where:** `internal/config/config.go:78-100`  ·  effort S  ·  confidence high
- **Impact:** User-facing safety defaults silently flip: redirects stop being followed, the CORS advisory disappears, and requests run with an unlimited timeout — the opposite of the documented first-run defaults. A user who upgrades from an older build, or whose config predates the settings block, gets degraded behavior with no error and no UI cue.
- **Fix:** In Load(), seed defaults before unmarshal so YAML only overwrites keys present in the file:  	c := Default() 	c.Workspaces = nil // let the file fully own the workspace list; re-default below if empty 	if err := yaml.Unmarshal(data, &c); err != nil { 		return Config{}, err 	}  A cleaner, intent-revealing variant: keep `var c Config`, unmarshal, then after the Workspaces/Active guards add an explicit Settings merge — e.g. if the unmarshaled Settings equals the zero value (or key fields like TimeoutSeconds==0 && Theme==\"\") replace with model.DefaultSettings(). Field-level merge is safest for partial settings blocks (a file with only `theme: d…

### [bug] OpenAPI import panics (nil-pointer) on a spec with no `info` block, crashing the whole app
- **Where:** `internal/importer/openapi.go:94-98`  ·  effort S  ·  confidence high
- **Impact:** importer.From is called directly in the file-open callback at internal/ui/import.go:85 with no recover(), and FromURL at import.go:102 likewise. A user opening a malformed/partial OpenAPI file (valid YAML/JSON, missing `info`) crashes the entire Fyne application — not a handled error dialog, a hard process exit.
- **Fix:** Guard the dereference in convertOAS3 (internal/importer/openapi.go:94-98): `name := "Imported API"; if doc.Info != nil { name = stringOr(doc.Info.Title, name) }` then set `Name: name`. Add a regression test in internal/importer feeding `{"openapi":"3.0.0","paths":{}}` asserting a collection named "Imported API" is returned with no panic. Per CLAUDE.md the test is mandatory and the importer package is under the >=90% coverage floor. Optionally also defend at the doc level (return an error if doc.Info == nil) if a missing info block should be rejected rather than silently defaulted — but the minimal nil-guard is sufficient to stop the crash.

### [bug] PrettyXML corrupts namespaced XML (incl. the importer's own SOAP envelopes) when used to Format a body
- **Where:** `internal/responsefmt/responsefmt.go:30-54`  ·  effort M  ·  confidence high
- **Impact:** Per responsefmt/WORKFLOW.md, PrettyXML now backs the request-body Format/Validate button. Clicking Format on any namespaced XML — including a SOAP envelope produced by this unit's own FromWSDL — silently rewrites the body into something the server will reject. SOAP/namespaced XML is the dominant XML-on-the-wire case, so this is a high-impact data-corruption bug.
- **Fix:** Don't round-trip through encoding/xml's encoder for pretty-printing. Either reformat by re-indenting the raw token stream while preserving original tag bytes, pull in a namespace-preserving formatter, or at minimum document the limitation and refuse (return an error) when xmlns prefixes are present so the user isn't handed corrupted XML silently. Add tests pinning a namespaced doc and a CDATA doc.

### [bug] Add/Rename/Delete/Duplicate persist the ACTIVE collection, not the one they mutated
- **Where:** `internal/session/items.go:30-31,60-61,81-82,134,259,280-281,293-294`  ·  effort M  ·  confidence high
- **Impact:** Latent today because the tree's OnSelected sets active to the selected node's collection (shell.go:546-547), but the contract is fragile: any flow that acts on a node without first selecting it (e.g. parentForNew falling back after lastSelectedNodeID is cleared on delete, shell.go:957 / items.go:193) writes the wrong collection and drops the user's edit. Silent data loss.
- **Fix:** Make each mutator persist by the resolved collection index instead of the active one, mirroring MoveNode. Compute ci = nodeCollectionIndex(parentID) (for Add*) or nodeCollectionIndex(nodeID) (for Rename/Delete/Duplicate) and call s.saveCollection(ci) in place of s.SaveActiveCollection() at items.go:30, 60, 81, 134, 259, 280, 293. The helpers already exist (saveCollection at session.go:755, nodeCollectionIndex at items.go:179). For AddRequestValue, drop the doc-comment requirement that the caller make parentID's collection active first, since persistence would then be index-resolved. Add a regression test in internal/session covering a mutatio…

### [bug] RemoveCollection deletes the wrong workspace entry when an earlier collection failed to load
- **Where:** `internal/session/session.go:168-184`  ·  effort S  ·  confidence high
- **Impact:** With a missing/corrupt collection dir in the workspace, removing a collection from the sidebar silently drops the wrong one from the persisted config — data/state loss that survives restart.
- **Fix:** Resolve the removal by dir, not by raw index: read dir := s.dirs[i] (the cols-aligned dir the UI actually targeted), then delete that dir from w.Collections via slices.Index/slices.Delete. Or mirror MoveCollection's len(w.Collections) != len(s.cols) misalignment guard and translate the cols index to the matching w.Collections index by dir before deleting.

### [concurrency] Leaf request's Chain slice is shared with m.currentRequest and read on the worker goroutine — data race
- **Where:** `internal/ui/shell.go:1045-1047, 1127`  ·  effort S  ·  confidence high
- **Impact:** Classic Go data race: torn reads of ChainStep fields, or reading past a shrunk slice's logical length, while the runner walks the chain. Undefined behaviour; in practice can mis-resolve or mis-execute a chain step, and trips the race detector (which AGENTS.md mandates via `go test ./... -race`).
- **Fix:** Deep-copy the leaf's slice-backed fields at Send entry on the UI thread, mirroring cloneRequestForChain in session.go. At minimum copy Chain: `req.Chain = append([]model.ChainStep(nil), m.currentRequest.Chain...)`. Ideally clone Params/Headers/Body.Form there too (see related finding) rather than relying on ExecuteOnce's late copy. Add a -race test that starts a send and mutates currentRequest.Chain concurrently.

### [concurrency] ~ Send() snapshots the request with aliased slices; leaf deep-copy is deferred into the worker goroutine, racing UI-thread edits
- **Where:** `internal/ui/shell.go:1045-1059, 1106-1149, 85-90`  ·  effort S  ·  confidence high
- **Impact:** Data race on KeyValue struct fields (and slice headers under append-growth) between the worker's deep-copy/read and UI-thread edits while a Send/chain is in flight. Detectable by -race; can produce torn reads or a send that captures a half-typed key/value. Violates the spirit of invariant 4's careful off-UI snapshotting.
- **Fix:** Move the leaf request's slice deep-copy onto the UI thread, at snapshot time in send(), before the `go func()` is launched. After `req = *m.currentRequest` (shell.go:1047) and the Auth flatten (1050), add: `req.Params = append([]model.KeyValue(nil), req.Params...)`, `req.Headers = append([]model.KeyValue(nil), req.Headers...)`, `req.Body.Form = append([]model.KeyValue(nil), req.Body.Form...)` (or call a UI-thread snapshot helper mirroring session.deepCopyRequest at items.go:547, minus the ID regeneration). Then the worker (and chainExecutor.ExecuteOnce) only ever sees a private copy. Drop the now-redundant copy at shell.go:88-90, OR keep it (…

### [security] Script timeout does not preempt native built-ins — a one-line script can OOM-crash Helena
- **Where:** `internal/scripting/bindings.go:109-150`  ·  effort M  ·  confidence high
- **Impact:** An imported/shared collection (the documented threat model — collections are executable) or even an honest typo can crash the entire app or freeze it past the advertised 5s cap, losing the user's in-flight work. The advertised sandbox resource cap is bypassable with a single builtin call.
- **Fix:** Don't run the script inline on the worker goroutine and rely solely on Interrupt. Run vm.RunString in its own goroutine and, on timeout/ctx-cancel, both call vm.Interrupt AND abandon the goroutine (return the timeout error to the caller without waiting for the native call to unwind) so the UI/worker isn't held hostage. Additionally cap goja memory/allocation where possible (goja exposes no hard memory limit, so at minimum bound the largest amplifiers: reject/limit response Body size before binding — see related finding — and document that native built-ins are not interruptible). At minimum, update AGENTS.md invariant 13 and README 'Timeout' s…

## MEDIUM (12)

### [bug] cURL/wget export silently drops a user-set Host header, breaking fidelity for vhost-targeted requests
- **Where:** `internal/exporter/exporter.go:54-58,82-86`  ·  effort S  ·  confidence high
- **Impact:** WORKFLOW.md's stated fidelity guarantee ("the bytes/URL that would go on the wire are what the command produces") is violated. A user routing to a backend by Host header gets an export that hits the wrong virtual host with no warning — confusing and potentially a wrong request against production.
- **Fix:** After Build, if req.Host != "" and differs from req.URL.Host, emit `-H 'Host: <req.Host>'` (curl) / `--header='Host: <req.Host>'` (wget). Add an exporter test with a Host header asserting the line appears.

### [bug] ~ DuplicateItem shares the Chain backing array between original and copy (aliasing bug)
- **Where:** `internal/session/items.go:547-559`  ·  effort S  ·  confidence high
- **Impact:** Duplicating a request (or a folder containing chained requests) produces a copy whose Chain aliases the original. A subsequent rename/move cascade or any in-place chain edit corrupts both, leading to wrong before-hook execution. Data-integrity bug in a user-visible feature.
- **Fix:** In internal/session/items.go deepCopyRequest, add a Chain clone alongside the existing slice clones (ChainStep is all-string value struct, so a shallow clone fully detaches): `if r.Chain != nil { r.Chain = slices.Clone(r.Chain) }`. This matches the established pattern in cloneRequestForChain (session.go:493). Add a regression test in internal/session/items_test.go that duplicates a request whose Chain is non-empty, then mutates the original's Chain[0].Request in place and asserts the copy's Chain[0].Request is unchanged (and extend TestDuplicateFolderDeepCopies to assert Chain independence for nested chained requests). Per CLAUDE.md "tests ar…

### [bug] RemoveCollection's 'no active workspace' guard tests the wrong field and blocks valid removals
- **Where:** `internal/session/session.go:169-171`  ·  effort S  ·  confidence high
- **Impact:** Unloadable collections become un-removable, and the error text misattributes the cause. Combined with finding #1, the remove path is unreliable precisely in the degraded states it most needs to handle.
- **Fix:** Replace the activeCol guard at session.go:169-171 with a real workspace-validity check mirroring MoveCollection (session.go:770): `if s.cfg.Active < 0 || s.cfg.Active >= len(s.cfg.Workspaces) { return fmt.Errorf("no active workspace") }`. This both fixes the false refusals (the i-range check at line 173 already bounds the removal) and properly guards the line-172 `s.cfg.Workspaces[s.cfg.Active]` dereference. Add a regression test: open a workspace, SetActiveCollection(-1) (or simulate all-collections-failing-to-load), then assert RemoveCollection(validIndex) succeeds. Per AGENTS.md the existing -1/99 out-of-range assertions still hold under t…

### [bug] Query/path param `type` discriminator is dropped on round-trip (path params become query params)
- **Where:** `internal/storage/opencollection.go:169-171`  ·  effort M  ·  confidence high
- **Impact:** Bruno-authored or hand-edited path parameters silently change semantics on the first Helena save. This is a data-fidelity regression against the OpenCollection format and partially undermines invariant 1's intent (a meaningful, spec'd field is lost even though it isn't strictly an 'unknown' key).
- **Fix:** Two options, in increasing robustness:  (a) Minimal / merge-only: in mergeParamExtras (store.go:230) also carry the prior row's Type when names match, e.g. after copying Extra: `if next[i].Type == "query" && prev had a Type, restore it`. Build a `map[string]string` of prior name->Type alongside the Extra map and apply it. Caveat: this only helps when a prior file exists at save time AND the param name is unchanged; a renamed param or a fresh save with no prior still loses the distinction. It also can't survive scripting round-trips since the model still has no Type.  (b) Correct / model-level (preferred): add a `Kind`/`Type` field to the para…

### [bug] Dragging a folder onto the top half of a collection root produces dstParent="" and fails with a misleading "cross-collection" error
- **Where:** `internal/ui/treedrag.go:121-124`  ·  effort S  ·  confidence high
- **Impact:** A natural gesture (drag a folder to the top of its collection to make it the first folder) silently fails with a confusing, wrong error message. The folder stays put. This is a user-visible correctness defect in the core DnD feature.
- **Fix:** When the target is a collection root, dstParent must be the collection root ID itself, not splitNode's empty parent. Either special-case it (if tgtParent == "" { dstParent = tgtID; index = 0 }) or, more cleanly, route this case to dstParent: tgtID with index 0 — i.e. treat "before the first folder of a collection root" as "into the root at index 0". Add a regression test (the folder-onto-collection-root-top-half scenario is currently untested; existing TestPlanNodeDrop only covers folder-before-folder where both share a non-root parent).

### [code-quality] Chain alias/ref delete-and-add paths re-derive RequestID on every rebuilt row via SetText OnChanged
- **Where:** `internal/ui/chain.go:78-113`  ·  effort S  ·  confidence medium
- **Impact:** An ID-pinned chain ref whose target was renamed (invariant 14's rename-survival contract) can lose its pin as a side effect of editing an unrelated row, after which resolution falls back to the now-stale path. Narrow but contradicts the ID-first survival guarantee.
- **Fix:** Wrap the row rebuild on add/delete in the m.loading guard so seeding existing values never re-runs RequestID derivation, mirroring loadRequest. Simplest: guard inside rebuildChainRows itself so all non-load callers (addChainStep, delete button) are covered:  func (m *MainUI) rebuildChainRows() {     prev := m.loading     m.loading = true     defer func() { m.loading = prev }()     m.chainRows.RemoveAll()     if m.currentRequest != nil {         for i := range m.currentRequest.Chain {             m.chainRows.Add(m.buildChainRow(i))         }     }     m.chainRows.Refresh() }  Save/restore (not bare false) is important because rebuildChainRows…

### [concurrency] ~ Leaf Params/Headers/Body.Form slices stay shared until the leaf ExecuteOnce, racing UI-thread KV edits
- **Where:** `internal/ui/shell.go:1047, 1088-1090, 1149`  ·  effort S  ·  confidence high
- **Impact:** Same data-race class as the Chain finding: torn KeyValue reads or out-of-bounds-by-logical-length reads when the leaf is finally built. Detected by -race.
- **Fix:** Deep-copy the leaf's mutable slices into `req` on the UI thread at Send entry, immediately after the shallow copy at shell.go:1047 (and before the `go func()` at 1106), mirroring what ExecuteOnce already does:    req = *m.currentRequest   req.Params  = append([]model.KeyValue(nil), req.Params...)   req.Headers = append([]model.KeyValue(nil), req.Headers...)   req.Body.Form = append([]model.KeyValue(nil), req.Body.Form...)   req.Auth = m.sess.EffectiveAuth(m.currentRequestID)  This insulates the leaf identically to chain steps (which already come from the isolated SnapshotChainFinder). The ExecuteOnce copy at shell.go:88-90 can stay as defense…

### [docs] cmd/helena docs (STRUCTURE.md + WORKFLOW.md) omit the tooltip layer added to main.go
- **Where:** `cmd/helena/STRUCTURE.md, cmd/helena/WORKFLOW.md, cmd/helena/main.go:STRUCTURE.md:43-50; WORKFLOW.md:21-23; main.go:48`  ·  effort S  ·  confidence high
- **Impact:** Per-module docs no longer match the entrypoint; a contributor tracing startup (the docs' stated purpose) will not learn that the content is tooltip-wrapped, and the line anchors point at the wrong code. The redesign change is, by the repo's own rule, 'not done' until these are updated.
- **Fix:** In WORKFLOW.md, change the startup line to reflect the wrap, e.g. `-> w.SetContent(fynetooltip.AddWindowToolTipLayer(mainUI.Root(), w.Canvas()))  (tooltip layer for icon-only toolbar)`, and add fyne-tooltip to the dependency narrative. In STRUCTURE.md, rewrite step 8 ("Install content") to describe AddWindowToolTipLayer wrapping the root with the window canvas and re-anchor it to main.go#L48 (note the inline comment at L46-47). While in there, re-anchor the rest of the stale references in one pass since they are all off: SetOnStopped -> L50-L53, ShowAndRun -> L55, plus step 2 (L19-20), step 5 (L33), step 6 (L35-42), and step 7 (L44-45). Optio…

### [error-handling] writeYAML is not atomic — a crash or power loss mid-write truncates/corrupts a collection file
- **Where:** `internal/storage/store.go:484-491`  ·  effort M  ·  confidence high
- **Impact:** Data loss / unloadable collection on an ill-timed crash during save. Especially bad for opencollection.yml: corrupting it makes Load return an error for the whole collection (store.go:330-336).
- **Fix:** Write to a temp file in the same directory and os.Rename it over the target (atomic on POSIX; on Windows use os.Rename with a remove-then-rename or a rename-replace helper). Apply uniformly via writeYAML.

### [performance] ~ Unbounded response body flows into the script runtime (string copy + JSON/XML parse)
- **Where:** `internal/scripting/bindings.go:397-432`  ·  effort M  ·  confidence high
- **Impact:** A large (or hostile) response body causes a large transient allocation + double parse on every post-script run, blocking the Send worker for the duration with no timeout coverage. The chain path already uses lazy accessors (lazyResponseToObject) to avoid this, but the top-level response global does not.
- **Fix:** Primary fix: cap the response body at read time in internal/httpclient/httpclient.go:194 with `io.LimitReader(resp.Body, max)` (configurable via Client settings, with a sane default), so an unbounded/hostile body can never be materialized into a string or fed to the VM. This is the load-bearing mitigation and protects every consumer (scripts, pretty view, storage), not just post-scripts.  Secondary (optional, smaller win): make the top-level response.json / response.xml lazy by reusing the DefineAccessorProperty pattern from lazyResponseToObject, so the JSON/XML parse only runs when a script actually reads those globals. Note this does NOT re…

### [security] ~ Response body read with unbounded io.ReadAll — large/malicious response can OOM the app
- **Where:** `internal/httpclient/httpclient.go:194-210`  ·  effort M  ·  confidence high
- **Impact:** Denial of service / crash of the user's client from a single oversized response. For a desktop API client this is a realistic foot-gun (e.g. hitting a file-download endpoint).
- **Fix:** Wrap the body in an io.LimitReader with a configurable max (e.g. derived from Settings, default ~50-100 MB). When the limit is hit, truncate and set a flag/warning on Response (similar to CORSWarning) so the UI can tell the user the body was truncated. Add a test that a body exceeding the cap is truncated rather than fully buffered.

### [testing] session package is below the 90% coverage floor; move/cascade error paths untested
- **Where:** `internal/session/items.go:309-461`  ·  effort M  ·  confidence high
- **Impact:** Below-floor coverage is an explicit invariant violation. More importantly the untested branches are the move/cascade edge cases most likely to harbor the aliasing and persistence bugs above; they would have caught both.
- **Fix:** Add targeted tests in internal/session/move_test.go (and one in items_test.go for DuplicateItem error paths): 1. Move that triggers a chain-ref cascade: set up request A with a Chain step referencing a request by name-path under a folder, then move/rename that folder's path via MoveNode and assert the Chain ref was rewritten (covers items.go:392-394, rewriteRequests, hasPrefix true/false branches). 2. clampIndex boundaries: move with index < 0 and index > len(dst) and assert the item lands at 0 / end (covers 405-410). 3. ID backfill on a legacy folder/request without model.ID, including via folderIDForContainer when the destination folder has…

## LOW (77) — one-liners grouped by category

**architecture (2)**

- ~`internal/ui/shell.go:307-332` — Form-urlencoded / multipart body types selected in the editor have no Form-fields editor
- ~`internal/ui/tabs.go:210-223` — closeAllTabs leaves the persisted tab set pointing at the previous workspace's collections

**bug (18)**

- ~`internal/auth/auth.go:130-134` — API-key query placement re-encodes the entire RawQuery via url.Values, dropping params that don't round-trip and reordering keys
- ~`internal/auth/oauth2.go:32-36, 154-171, 247-251; internal/auth/oauth2_authcode.go:56-59,152-157` — refresh_token is cached but never used; expired tokens re-run the full grant (re-opens browser for auth_code)
- `internal/auth/oauth2.go:241-251` — Default expires_in of 3600s is applied even when the IdP explicitly returned a non-positive / unparseable value, risking use of a token past its real lifetime
- `internal/chain/chain.go:194-199, 224-226` — Cycle detection is skipped entirely for requests with empty IDs, with no guard at the package boundary
- ~`internal/httpclient/httpclient.go:126-132` — Query merge re-encodes and reorders pre-existing URL query params via q.Encode()
- ~`internal/httpclient/httpclient.go:251-259` — corsAdvisory treats Access-Control-Allow-Origin: * as always-OK, ignoring the credentials case
- ~`internal/scripting/bindings.go:297-302` — Non-deterministic ordering of script-added headers/params/form fields
- ~`internal/session/items.go:195-222` — Chain-ref cascade rewrites by name-path prefix only, corrupting refs to unrelated same-named nodes
- ~`internal/session/session.go:149-161` — OpenCollection allows the same directory to be opened multiple times
- ~`internal/storage/store.go:93-145` — Save mis-pairs Extra/info.id/Tags to the wrong item when two siblings share a slug and are reordered
- ~`internal/storage/store.go:467-481, 387-394` — loadItems / loadEnvironments ordering is non-deterministic when info.seq is missing or duplicated
- `internal/ui/chain.go:118-137` — chainRefSuggestions self-cycle filter can hide a legitimate different-folder request that shares the editing request's name
- ~`internal/ui/helena_theme.go:180-196` — splitTheme leaves ColorNameHover unoverridden, so the thin divider line nearly vanishes on hover
- `internal/ui/query.go:18-23, 28-45` — splitURLQuery does not strip URL fragments; a '#fragment' is absorbed into the last query param value
- ~`internal/ui/tabs.go:69-76, 85, 519` — Tab dedup ignores owning collection, breaking the documented (collection, requestID) identity for collections sharing a Request.ID
- `internal/ui/tabs.go:628-644` — commitScratchTab switches the active collection before the save can fail, leaving stale active-collection/env state on error
- `internal/ui/workspaces.go:29-37, 88-95` — Workspace names are not unique; the name-keyed Workspace dropdown can never select the second of two same-named workspaces
- ~`internal/vars/vars.go:11-12, 53-63` — Resolver chain deeper than maxPasses (10) silently fails and is mis-reported as missing

**code-quality (10)**

- ~`internal/chain/chain.go:260-289, 204-209` — Progress `total` can under-report when a legitimate chain exceeds MaxChainSteps, showing 'step N/32' on a chain that then errors
- `internal/scripting/bindings.go:86-102` — console output JSON-fallback prints Go pointers/garbage for unmarshalable values
- `internal/session/items.go:535-544` — parseLeaf accepts unknown leaf kinds, returning a non-c/f/r kind as ok=true
- ~`internal/session/session.go:255-272` — SetActiveEnv pollutes the in-memory env map with a -1 key when no collection is active
- ~`internal/ui/auth.go:120-123` — Auth tab "Clear cached tokens" calls ClearAll(), nuking every collection's OAuth2 tokens instead of this request's
- `internal/ui/export.go:61-72, 39-46` — Export snippet entries are editable widget.Entry instances, not read-only
- ~`internal/ui/helena_theme.go:121-229` — Five near-identical pass-through sub-theme structs duplicate Color/Font/Icon/Size delegation boilerplate
- ~`internal/ui/shell.go:611-650` — loadRequest silently rewrites the live in-memory request URL into Params on open (reformats on next save)
- ~`internal/ui/tabs.go:471-485` — persistTabs records active index 0 when the active tab is a scratch (unpersistable) tab
- `internal/ui/treedrag.go:129-141` — Drop indicator for a folder-onto-request drop is misleading: line is drawn at the request, folder lands at end of the folder slice

**concurrency (2)**

- ~`internal/auth/oauth2.go:154-171; internal/auth/oauth2_authcode.go:42-158` — Token() check-then-fetch is not atomic per key — concurrent sends each run the full grant; for authorization_code this opens duplicate browser tabs and binds duplicate listeners
- ~`internal/chain/chain.go:167-228` — `resolveSteps` never checks `ctx.Done()` between steps; mid-chain abort waits for the current step to notice cancellation

**docs (6)**

- `internal/auth/auth.go:10-12, 25-27, 96-99` — ErrOAuth2NotImplemented doc comment and package doc are stale — OAuth2 is implemented; the sentinel now means 'unsupported grant', not 'not wired'
- `internal/chain/WORKFLOW.md, internal/chain/README.md, internal/chain/STRUCTURE.md:WORKFLOW.md:5,7; README.md:212; STRUCTURE.md:134` — WORKFLOW.md / README.md / STRUCTURE.md reference a non-existent `sessionRequestFinder` type and stale Resolve signature
- `internal/exporter/WORKFLOW.md:7` — exporter WORKFLOW.md documents the wrong httpclient.Build signature (3 args, actual is 4)
- `internal/scripting/WORKFLOW.md:10-13, 124-133` — WORKFLOW.md is stale: wrong env-bridge description and 'chains not yet shipped' claim
- `internal/ui/helena_theme.go:254-264` — ForegroundOnError/Success/Warning contrast colors are silently delegated while the doc claims the accent set must be explicit
- `internal/ui/treedrag.go:42-50` — dropTreeNode doc comment claims it applies a "cached" plan but it recomputes

**error-handling (13)**

- `internal/auth/oauth2.go:123-134, 179-205` — Token-endpoint client_credentials POST inherits the caller ctx but the wired resolver uses http.DefaultClient (no timeout) — a hung token endpoint can wedge a Send
- ~`internal/httpclient/httpclient.go:237-238` — Multipart body unsupported but reported via fmt.Errorf rather than a sentinel — callers can't branch on it
- ~`internal/session/items.go:388-401` — MoveNode and the other mutators leave memory diverged from disk when Save fails
- ~`internal/session/session.go:153-155` — OpenCollection indexes s.cfg.Workspaces[s.cfg.Active] without the bounds guard its siblings carry
- `internal/session/session.go:77-87` — reload() silently drops collections that fail to load with no surfaced diagnostic
- ~`internal/storage/store.go:45-74, 146-173` — Save aborts mid-tree on first write/sweep error, leaving a partially-updated on-disk collection
- `internal/ui/shell.go:1108-1117` — Panic-recovery teardown skips deliverResponse, leaving the originating tab's cached response stale
- ~`internal/ui/shell.go:1127-1140, 1149-1190` — Aborted chain leaves the in-flight chain step's overlay writes rolled back, but a successful chain followed by leaf post-script failure does not roll back
- ~`internal/ui/shell.go:738-784` — validateBody / formatBody operate on unresolved {{var}} body text, flagging valid bodies as invalid
- `internal/ui/shell.go:1323-1326` — editSettings silently coerces invalid / negative timeout input to 0 (no timeout)
- `internal/ui/tabs.go:532-539` — A response delivered to a tab closed mid-Send is silently dropped
- `internal/ui/theme.go:22-31` — variantFor lacks the nil-app guard that appTheme has, so it can panic where appTheme would degrade gracefully
- `internal/ui/treedrag.go:203-234` — showDropIndicator dereferences the driver without the nil guard used by rowAt

**performance (10)**

- `internal/chain/chainvars.go:86-114` — `lookupJSONPath` re-decodes the entire response body on every `{{chain.<alias>.response.json.*}}` reference
- `internal/httpclient/httpclient.go:60-73` — Each Send constructs a new Client/Transport — no connection reuse and idle connections accumulate
- `internal/importer/openapi.go:122-123` — pathItem.Operations() rebuilt twice per path during OpenAPI conversion
- ~`internal/ui/helena_theme.go:59-61` — themedIcon allocates a fresh embedded resource on every call despite assets.Icon's explicit 'cache for hot paths' guidance
- ~`internal/ui/import.go:69-95, 97-142` — importFromFile parses the spec on the UI goroutine; importFromURL offloads it — inconsistent, can freeze the UI
- `internal/ui/shell.go:1061-1068` — send() builds a fresh httpclient + OAuth2 resolver on every call instead of reusing per-session clients
- `internal/ui/shell.go:864-869, 275-280` — applyURLEdit rebuilds the entire Params row container on every URL keystroke
- ~`internal/ui/tabs.go:362-371, 379-397` — Drag reorder allocates two slices on every DragEvent even when the order is unchanged
- `internal/ui/treedrag.go:145-171` — applyDrop runs synchronous YAML save (Session.MoveNode/MoveCollection) on the UI goroutine
- ~`internal/vars/vars.go:65-76` — substituteOnce runs the regex twice per match (FindStringSubmatch inside ReplaceAllStringFunc)

**security (7)**

- ~`internal/auth/oauth2.go:99-101` — CacheKey omits ClientSecret, RedirectURI, UsePKCE and Grant-specific inputs — rotated secret or changed config silently reuses a stale token
- ~`internal/auth/oauth2.go:210-213; internal/auth/oauth2_authcode.go:242-244` — Token-endpoint error bodies are embedded verbatim into errors that bubble to the UI — risks surfacing/logging sensitive token-endpoint output
- ~`internal/auth/oauth2_authcode.go:87-134` — Local callback listener accepts an unauthenticated first request from any local process; only state (not a per-request nonce binding) gates the exchange
- ~`internal/httpclient/httpclient.go:188-191` — Network error from c.http.Do is returned raw and leaks the resolved URL (with secrets) into UI status text
- ~`internal/httpclient/httpclient.go:126-138` — Custom auth headers (API-key-in-header) survive cross-host redirects — credential leak on redirect
- `internal/importer/url.go:33-37` — FromURL reads an unbounded response body from an untrusted URL with no size cap
- ~`internal/vars/vars.go:53-76` — Resolver re-expands substituted values, enabling variable-content injection

**testing (9)**

- `internal/chain/chain.go:48, 184-186` — Alias validation accepts JS reserved words; test comment overstates the check
- `internal/httpclient/httpclient.go:194-198` — No test coverage for the response-body read-error path
- ~`internal/importer/openapi_test.go:1-359` — No regression tests for the malformed-input paths (nil Info panic, FromURL non-2xx/oversize)
- `internal/storage/store.go:93-145` — No regression tests for slug-collision Extra mispairing or path-param type loss; coverage sits at the 90% floor
- `internal/ui/helena_theme_test.go:1-170` — No tests for splitTheme, paneTheme, rootTheme, refreshThemedCanvas, or themedIcon
- ~`internal/ui/method.go:1-128` — method.go, import.go, export.go, collections.go, workspaces.go, oauth2.go have no dedicated UI tests for their helpers
- `internal/ui/shell.go:672-707, 738-784, 864-869` — No UI-level tests for saveRequest, validateBody/formatBody, applyURLEdit/applyImpliedContentType integration, or the send() race window
- ~`internal/ui/shell_test.go:43-102` — No end-to-end test of the async Send goroutine, deliverResponse-from-worker, abort, or chain execution
- `internal/vars/vars.go:67-70` — No test covers an empty / whitespace-only {{}} template (uncovered substituteOnce branch)

