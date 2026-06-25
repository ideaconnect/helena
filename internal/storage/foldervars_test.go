package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestFolderVariablesRoundTrip pins #81: folder-scoped variables survive
// Save->Load (order, enabled flag, secret flag) for a nested folder, and a
// Secret value is externalized — never written in cleartext to the folder YAML.
func TestFolderVariablesRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	outerVars := []model.Variable{
		{Enabled: true, Key: "region", Value: "eu"},
		{Enabled: true, Key: "key", Value: "outer-s3cret", Secret: true},
	}
	innerVars := []model.Variable{
		{Enabled: false, Key: "debug", Value: "1"},
		{Enabled: true, Key: "scope", Value: "admin"},
	}
	col := model.Collection{
		Name: "C",
		Folders: []model.Folder{{
			Name:      "Outer",
			Variables: outerVars,
			Folders: []model.Folder{{
				Name:      "Inner",
				Variables: innerVars,
				Requests:  []model.Request{{Name: "R", Method: model.GET, URL: "https://x/"}},
			}},
		}},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}

	// The secret value must not appear in any YAML file under the collection.
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yml") {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), "outer-s3cret") {
				t.Errorf("folder-variable secret leaked into %s:\n%s", path, b)
			}
		}
		return nil
	})

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotOuter := got.Folders[0].Variables
	gotInner := got.Folders[0].Folders[0].Variables
	if !reflect.DeepEqual(gotOuter, outerVars) {
		t.Errorf("outer folder vars did not round-trip:\n got  %+v\n want %+v", gotOuter, outerVars)
	}
	if !reflect.DeepEqual(gotInner, innerVars) {
		t.Errorf("inner folder vars did not round-trip:\n got  %+v\n want %+v", gotInner, innerVars)
	}
}
