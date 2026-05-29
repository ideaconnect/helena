# examples — workflows

## Loading the bundled sample

This is the single flow the package exercises. It runs during
`go test ./examples/`.

1. **Test setup.** `package examples_test` imports `internal/storage`. The
   Go test runner sets the working directory to the package directory
   (`examples/`) before running tests.
2. **Load.** `storage.Load("httpbin")`
   ([example_test.go:13](example_test.go#L13)) resolves the `httpbin`
   directory relative to the cwd, opens
   [httpbin/opencollection.yml](httpbin/opencollection.yml), and reads the
   collection's name and type.
3. **Enumerate requests.** Storage walks the directory for `*.yml` files
   other than `opencollection.yml` and parses each as a request. The two
   files in the sample are
   [httpbin/get-anything.yml](httpbin/get-anything.yml) and
   [httpbin/post-anything.yml](httpbin/post-anything.yml). Their `info.seq`
   values (1 and 2) determine display order.
4. **Enumerate environments.** Storage descends into
   `httpbin/environments/` and parses each `*.yml` into a
   `model.Environment`. Only
   [httpbin/environments/default.yml](httpbin/environments/default.yml) is
   present.
5. **Assert.** The test checks:
   - `c.Name == "httpbin sample"` (matches `info.name`).
   - `len(c.Requests) == 2` (the two `*-anything.yml` files).
   - Methods map to `{"GET /anything": "GET", "POST /anything": "POST"}`.
   - `len(c.Environments) == 1` and the env's name is `default`.
   - The env contains `base_url == https://httpbin.org` so request
     substitution works against a real upstream.

If any of these assertions fail, the on-disk schema or the storage layer
has drifted and needs reconciling. The sample also doubles as a ready-made
collection for human contributors: open `examples/httpbin/` in Helena to
explore the layout.
