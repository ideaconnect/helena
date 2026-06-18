# storage — workflows

This document walks through the load and save flows and the key invariant
behind both: the `Extra` round-trip that keeps externally-authored fields
intact across edits.

## The Extra round-trip — why every save reads first

Every DTO in [opencollection.go](opencollection.go) has an
`Extra map[string]yaml.Node` field tagged `yaml:",inline"`. On unmarshal, any
YAML key not explicitly named on the struct lands in `Extra`. On marshal, the
`,inline` tag splices those keys back into the parent mapping verbatim.

That alone preserves unknown fields through a parse-then-write of a single
file. But the in-memory `model.*` types do not carry `Extra` — they only model
the fields Helena understands. The bridge is that `Save` reads the existing
file from disk before writing the new one, copies `Extra` (and other unmapped
fields like `Info.Tags` and `Info.Extra`) from the old DTO into the new DTO,
and then writes. Concretely, for a request file:

```
new := requestToFile(r, seq)            // built from model
prev, _ := readRequestFile(path)        // read existing file from disk
new.Extra        = prev.Extra            // copy top-level unknowns
new.Info.Extra   = prev.Info.Extra       // copy info-block unknowns
new.Info.Tags    = prev.Info.Tags        // copy unmodelled tags
if new.HTTP != nil && prev.HTTP != nil {
    new.HTTP.Extra = prev.HTTP.Extra     // copy http-level unknowns (auth, …)
}
writeYAML(path, new)                    // re-marshal with Extra inlined
```

The same pattern applies to `opencollection.yml`, `folder.yml`, environment
files, and the per-request file: every writer reads its prior file first if it
exists, layers Extra on top, and then re-marshals.

This means edits performed through Helena — renaming a request, changing a
header, adding a body — only touch the fields Helena understands; everything
else in the file is bit-for-bit preserved (subject to YAML re-serialization
of the keys Helena knows).

## Saving a collection (preserving Extra)

`Save(c model.Collection, dir string)` is **atomic at the tree level** (#109)
and **externalizes secrets** (#42). Before staging, `splitSecrets` produces a
sanitized deep copy of `c` with every secret field blanked (Basic password,
Bearer token, API-key value, OAuth2 client secret, Secret env-var values, and
Secret collection-variable values, #80), collecting the real values into a map
keyed positionally
(`col/auth/bearer.token`, `f0/r1/auth/oauth2.clientSecret`, `e0/v1`, `cv0`, …).
`writeSecrets` persists that map to a per-collection file under the OS config
dir (`$HELENA_SECRETS_DIR` overrides; `secretsDirOverride` in tests), named by a
hash of the absolute collection dir — outside any repo, so it can never be
git-committed. The sanitized copy (no cleartext secrets) is what gets staged.
The secret store is written **first**, and the live in-memory model keeps its
real values, so a later save failure loses no credential.

The staging itself never writes into `dir` directly; instead it:

1. Seeds a sibling staging dir `<dir>.helena-save` with a copy of the current
   on-disk tree (`copyTree`) — so the per-file Extra round-trip below can still
   read prior files — or creates it empty on a first save.
2. Runs the full save logic (`saveInPlace`, below) against the staging dir.
3. Swaps atomically: renames `dir` aside to `<dir>.helena-old`, renames the
   staging dir into place as `dir`, then removes the old tree. If the second
   rename fails the old tree is renamed back, so `dir` is never left missing.

Any failure during steps 1–2 removes the staging dir and returns the error with
`dir` untouched — a half-written collection is impossible. The session layer
relies on this: on a save error it reloads from disk, and because disk is
unchanged the in-memory model rolls back to the last-good state.

`saveInPlace(c, dir)` (the staged target) does the following:

1. `os.MkdirAll(dir, …)`.
2. Build the root DTO: `ocCollectionFile{Info: ocInfo{Name: c.Name, Type: "collection"}}`.
3. Read the existing `opencollection.yml` if any; copy `Extra`, `Info.Extra`
   and `Info.Tags` onto the new DTO.
4. Write `opencollection.yml`.
5. For each environment in `c.Environments`:
   - Pick a slug via `slug(name, "env-<n>")`, deduplicated through
     `uniqueName` so two envs that slug to the same string get distinct
     filenames.
   - Build `ocEnvironmentFile` via `envToFile`.
   - Read the previous file at that slug and copy `Extra` and `Info.Extra`.
   - Write the file under `environments/`.
6. Sweep `environments/`: any `.yml` not produced this save is removed
   (folders inside `environments/` are not touched).
7. Call `saveItems(dir, c.Folders, c.Requests)`:
   - Walk requests; for each, slug + dedupe filename, build `ocRequestFile`
     via `requestToFile`, layer in `Extra` from any prior file (including
     `HTTP.Extra` which is where auth blocks live, `Scripts.Extra` which
     is where keys other tools nested inside the `scripts:` block live,
     and per-`Chain` entry `Extra` paired by alias so tool-authored
     description / metadata keys survive), write.
   - Walk folders; for each, slug + dedupe directory name, create the dir,
     build `ocFolderFile`, layer in `Extra` from prior `folder.yml`, write,
     and recurse into `saveItems(sub, f.Folders, f.Requests)`.
   - At the end of each container, call `sweepDir`.

## Loading a collection

`Load(dir string)` is the inverse, minus the Extra plumbing (loaded models do
not carry Extra; it only matters again on Save):

1. Read `opencollection.yml` to fetch the collection name; assign a fresh
   `model.NewID`.
2. `loadEnvironments(environments/)`:
   - Read every `.yml` (skip subdirectories), unmarshal as
     `ocEnvironmentFile`, convert with `fileToEnv`, attach the `info.seq` so
     environments can be re-ordered to match the file's stated order.
   - Sort by `seq` and return.
3. `loadItems(dir)`:
   - For each entry, if it's a subdirectory containing a `folder.yml`, parse
     it as `ocFolderFile`, recurse to load its children, append a
     `model.Folder` carrying the loaded children and a fresh ID.
   - Subdirectories without `folder.yml` are ignored (they are just user
     files alongside the collection).
   - For each `.yml` file other than `opencollection.yml` and `folder.yml`,
     parse it as `ocRequestFile`. If `info.type` is set and is not `"http"`,
     skip it (future-proofing for other request types). Otherwise convert
     with `fileToRequest`.
   - Sort folders and requests by their `info.seq` so on-disk order is
     preserved.
4. `applySecrets(&c, readSecrets(dir))` merges the externalized secrets (#42)
   back into the blanked fields, keyed positionally to match `splitSecrets`.
   Only **empty** fields are filled, so a pre-#42 collection that still carries
   a cleartext secret in its YAML loads unchanged and migrates to the store on
   its next save.

## Sweeping orphaned files on save

The sweep step is what makes deletions persist. The in-memory model has no
concept of "this used to exist" — when the user removes a request through the
UI, it just disappears from `c.Requests`. Without sweeping, the file would
linger on disk and reappear on the next `Load`.

`sweepDir(dir, keep)` walks `dir` once:

- Anything in `keep` is left alone (the active save just wrote those).
- Subdirectories: only deleted if they contain a `folder.yml`. Random user
  subdirectories without that marker are preserved.
- Files: only deleted if they end in `.yml`. Other files (`README.md`, hidden
  files, anything the user dropped in) are preserved.

The `keep` set is built up as the save runs, so it knows exactly which
filenames the just-saved collection produced:

- Top-level keeps `opencollection.yml`, `folder.yml`, `environments/`, plus
  every `<slug>.yml` and `<slug>/` produced by `saveItems`.
- `environments/` keeps every `<slug>.yml` produced by the env loop.

Subfolders run their own sweep at the end of `saveItems(sub, …)`.

## Slug & uniqueName

OpenCollection uses display names like `"Create User"`. The on-disk filename
is a slug derived from the name. `slug(name, fallback)` lowercases the name,
collapses runs of non-`[a-z0-9]` characters into `-`, trims leading and
trailing dashes, and falls back to e.g. `"request-3"` when nothing remains.

`uniqueName(base, used)` resolves slug collisions: if two requests both slug
to `"login"`, they are written as `login.yml` and `login-2.yml`. The
`used` set carries forward through the loop so the second name is recorded
before the third is generated.
