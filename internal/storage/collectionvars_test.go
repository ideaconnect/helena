package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestCollectionVariablesRoundTrip pins #80: collection-level variables survive
// Save→Load (order, enabled flag, secret flag) and a Secret value is
// externalized — never written in cleartext to the collection YAML (#42).
func TestCollectionVariablesRoundTrip(t *testing.T) {
	secretsDirOverride = t.TempDir()
	defer func() { secretsDirOverride = "" }()

	dir := t.TempDir()
	col := model.Collection{
		Name: "C",
		Variables: []model.Variable{
			{Enabled: true, Key: "base_url", Value: "https://api.test"},
			{Enabled: false, Key: "debug", Value: "1"},
			{Enabled: true, Key: "token", Value: "s3cret-value", Secret: true},
		},
	}
	if err := Save(col, dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, collectionFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "s3cret-value") {
		t.Errorf("collection-variable secret leaked into %s:\n%s", collectionFile, data)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Variables, col.Variables) {
		t.Errorf("collection variables did not round-trip:\n got  %+v\n want %+v", got.Variables, col.Variables)
	}
}
