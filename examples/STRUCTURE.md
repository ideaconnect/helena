# examples — structure

## Files

| File | Purpose |
| ---- | ------- |
| [sample.go](sample.go) | `package examples`. `//go:embed httpbin` bundles the sample into the binary; `WriteSample(destDir)` materializes it to `destDir/httpbin` and returns the path, and `SampleName` is that subdir name. Lets a downloaded binary load the sample with no source tree (#57). |
| [example_test.go](example_test.go) | `TestHttpbinSampleLoads` — smoke test that loads `httpbin/` via `storage.Load` and asserts the parsed collection name, request methods, environment name, and `base_url` variable. Lives in `package examples_test` so it depends on `internal/storage` from the outside. |
| [sample_test.go](sample_test.go) | `TestWriteSampleMaterializesAndLoads` — writes the embedded sample to a temp dir and loads it through `storage.Load`. |

## Bundled artifacts (httpbin/)

The directory is itself a Helena collection: opening `examples/httpbin/` in
the app loads it ready-to-use.

| Path | Role |
| ---- | ---- |
| [httpbin/opencollection.yml](httpbin/opencollection.yml) | Collection manifest. `info.name: httpbin sample`, `info.type: collection`. Required by `internal/storage` for a directory to be recognised as a collection. |
| [httpbin/get-anything.yml](httpbin/get-anything.yml) | `GET {{base_url}}/anything?lang=en` with `Accept: application/json`. `info.seq: 1` controls tree ordering. |
| [httpbin/post-anything.yml](httpbin/post-anything.yml) | `POST {{base_url}}/anything` with a `json` body containing a small object and `Content-Type: application/json`. `info.seq: 2`. |
| [httpbin/environments/default.yml](httpbin/environments/default.yml) | Environment named `default` with one variable, `base_url = https://httpbin.org`. The `{{base_url}}` references in the request YAMLs resolve through this env. |

## How the artifacts are consumed

- The test calls `storage.Load("httpbin")` — note the working directory is
  the `examples/` package directory at test time, so the relative path
  picks up the `httpbin/` subdirectory next to `example_test.go`.
- `storage.Load` reads `opencollection.yml`, then enumerates the directory
  for `*.yml` request files and the `environments/` subdirectory for
  environment files.
- The parsed `model.Collection` carries two `Requests` (sorted by `seq`)
  and one `Environment`.
- The test asserts the basics: name, request count, methods, env name, and
  the presence of the `base_url` variable. Any change to the on-disk
  schema that breaks this contract will fail the test.

There is no go:generate or codegen here — the YAML files are
hand-maintained.
