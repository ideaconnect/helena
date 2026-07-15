package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestPathParamsRoundTrip verifies path parameters survive Save -> Load, are
// kept separate from query params, and serialize with `type: path`.
func TestPathParamsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			Name:       "R",
			Method:     model.GET,
			URL:        "{{base_url}}/drs/bag/{bagId}",
			Params:     []model.KeyValue{{Enabled: true, Key: "q", Value: "x"}},
			PathParams: []model.KeyValue{{Enabled: true, Key: "bagId", Value: "42"}},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := got.Requests[0]
	if !reflect.DeepEqual(r.PathParams, col.Requests[0].PathParams) {
		t.Errorf("PathParams did not round-trip:\n got  %+v\n want %+v", r.PathParams, col.Requests[0].PathParams)
	}
	if !reflect.DeepEqual(r.Params, col.Requests[0].Params) {
		t.Errorf("Params changed:\n got  %+v\n want %+v", r.Params, col.Requests[0].Params)
	}

	// The on-disk file must carry the `type: path` discriminator so Bruno and
	// other OpenCollection tools read the parameter as a path param.
	data := readOnlyRequestYAML(t, dir)
	if !strings.Contains(data, "type: path") {
		t.Errorf("request YAML missing `type: path`:\n%s", data)
	}
}

// TestPathParamsResaveIsIdempotent pins the round-trip guarantee that matters
// for clean git diffs: once Helena has written a request, a Load -> Save cycle
// reproduces byte-identical YAML (path params serialize after query params in a
// stable, deterministic order). This is the fixed-point property; the query/path
// grouping is deliberate (see requestToFile).
func TestPathParamsResaveIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Requests: []model.Request{{
			// Pre-set the ID so the first save already carries it — otherwise Load
			// mints one that the second save persists (documented behavior), which
			// is unrelated to the param ordering this test pins.
			ID:         "fixed-request-id",
			Name:       "R",
			Method:     model.GET,
			URL:        "{{base_url}}/u/{userId}/o/{orderId}",
			Params:     []model.KeyValue{{Enabled: true, Key: "q", Value: "x"}, {Enabled: false, Key: "d", Value: "y"}},
			PathParams: []model.KeyValue{{Enabled: true, Key: "userId", Value: "7"}, {Enabled: true, Key: "orderId", Value: "9"}},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}
	first := readOnlyRequestYAML(t, dir)

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(loaded, dir); err != nil {
		t.Fatal(err)
	}
	if second := readOnlyRequestYAML(t, dir); second != first {
		t.Errorf("re-save was not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// TestBrunoPathParamLoadsSeparately verifies a hand-authored OpenCollection
// file whose param is `type: path` loads into PathParams, not Params — so it is
// filled into the URL rather than appended to the query string.
func TestBrunoPathParamLoadsSeparately(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, collectionFile), "info:\n  name: C\n  type: collection\n")
	writeFile(t, filepath.Join(dir, "r.yml"), strings.Join([]string{
		"info:",
		"  name: R",
		"  type: http",
		"http:",
		"  method: GET",
		"  url: https://api.example.com/bag/{bagId}",
		"  params:",
		"    - name: bagId",
		"      value: \"7\"",
		"      type: path",
		"    - name: q",
		"      value: x",
		"      type: query",
		"",
	}, "\n"))

	col, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := col.Requests[0]
	if len(r.PathParams) != 1 || r.PathParams[0].Key != "bagId" || r.PathParams[0].Value != "7" {
		t.Errorf("PathParams = %+v, want the type:path bagId", r.PathParams)
	}
	if len(r.Params) != 1 || r.Params[0].Key != "q" {
		t.Errorf("Params = %+v, want only the type:query q", r.Params)
	}
}

// readOnlyRequestYAML returns the contents of the single non-collection request
// file saved under dir's one collection folder.
func readOnlyRequestYAML(t *testing.T, dir string) string {
	t.Helper()
	var out string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".yml") && d.Name() != collectionFile {
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			out = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
