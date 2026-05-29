# examples

A small, real on-disk collection plus a smoke test that loads it through the
real `internal/storage` layer. The goal is to catch silent format drift: if
the storage code ever changes the on-disk YAML schema without updating the
sample, this test fails.

The bundled collection targets [httpbin.org](https://httpbin.org/) and
exercises a `GET` and a `POST` against `/anything`, plus a `default`
environment that defines `base_url`. The files live next to the test so
contributors can also open them in Helena to experiment without having to
hand-author a collection. There is no regeneration step — edit the YAML and
the icons by hand if you need to change them, and adjust
[example_test.go](example_test.go) to match.

## Files

- [example_test.go](example_test.go) — the smoke test
  (`TestHttpbinSampleLoads`).
- [httpbin/opencollection.yml](httpbin/opencollection.yml) — collection
  manifest (name + type).
- [httpbin/get-anything.yml](httpbin/get-anything.yml) — `GET /anything`
  with a `lang=en` query param and `Accept: application/json`.
- [httpbin/post-anything.yml](httpbin/post-anything.yml) — `POST /anything`
  with a JSON body.
- [httpbin/environments/default.yml](httpbin/environments/default.yml) —
  one environment carrying `base_url: https://httpbin.org`.
