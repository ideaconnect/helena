package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestRequestVariablesRoundTrip pins #82: request-scoped variables survive
// Save->Load (order, enabled flag, secret flag) for both a root request and a
// folder request, and a Secret value is externalized — never written in
// cleartext to the request YAML (#42).
func TestRequestVariablesRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	rootVars := []model.Variable{
		{Enabled: true, Key: "page", Value: "2"},
		{Enabled: false, Key: "debug", Value: "1"},
		{Enabled: true, Key: "token", Value: "req-s3cret", Secret: true},
	}
	folderVars := []model.Variable{
		{Enabled: true, Key: "id", Value: "42"},
		{Enabled: true, Key: "fkey", Value: "folder-s3cret", Secret: true},
	}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "Root", Method: model.GET, URL: "https://x/", Variables: rootVars}},
		Folders: []model.Folder{{
			Name:     "F",
			Requests: []model.Request{{Name: "Nested", Method: model.POST, URL: "https://y/", Variables: folderVars}},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}

	// Neither secret value may appear anywhere under the collection dir.
	walkAssertNoSecret(t, dir, "req-s3cret")
	walkAssertNoSecret(t, dir, "folder-s3cret")

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Requests[0].Variables, rootVars) {
		t.Errorf("root request variables did not round-trip:\n got  %+v\n want %+v", got.Requests[0].Variables, rootVars)
	}
	if !reflect.DeepEqual(got.Folders[0].Requests[0].Variables, folderVars) {
		t.Errorf("folder request variables did not round-trip:\n got  %+v\n want %+v", got.Folders[0].Requests[0].Variables, folderVars)
	}
}

// walkAssertNoSecret fails if needle appears in any *.yml file under root,
// confirming a secret value was externalized rather than written in cleartext.
func walkAssertNoSecret(t *testing.T, root, needle string) {
	t.Helper()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yml") {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), needle) {
			t.Errorf("secret %q leaked into %s:\n%s", needle, path, b)
		}
		return nil
	})
}
