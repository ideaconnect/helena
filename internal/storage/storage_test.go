package storage

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

func sampleCollection() model.Collection {
	return model.Collection{
		ID:   model.NewID(),
		Name: "Demo API",
		Requests: []model.Request{{
			ID:     model.NewID(),
			Name:   "Health",
			Method: model.GET,
			URL:    "https://{{base}}/health",
			Headers: []model.KeyValue{
				{Enabled: true, Key: "Accept", Value: "application/json"},
				{Enabled: false, Key: "X-Debug", Value: "1"},
			},
			Params: []model.KeyValue{{Enabled: true, Key: "verbose", Value: "true"}},
			Body:   model.Body{Type: model.BodyNone},
		}},
		Folders: []model.Folder{{
			ID:   model.NewID(),
			Name: "Users",
			Requests: []model.Request{{
				ID:     model.NewID(),
				Name:   "Create User",
				Method: model.POST,
				URL:    "https://{{base}}/users",
				Body:   model.Body{Type: model.BodyJSON, Content: "{\n  \"name\": \"Ada\"\n}"},
			}},
		}},
		Environments: []model.Environment{{
			ID:        model.NewID(),
			Name:      "Local",
			Variables: []model.Variable{{Enabled: true, Key: "base", Value: "http://localhost:8080"}},
		}},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	orig := sampleCollection()

	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	clearIDs(&orig)
	clearIDs(&got)
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n orig=%#v\n got =%#v", orig, got)
	}
}

func TestRequestFileUsesSpecKeys(t *testing.T) {
	rf := requestToFile(model.Request{
		Name:    "Create User",
		Method:  model.POST,
		URL:     "https://api/users",
		Headers: []model.KeyValue{{Enabled: false, Key: "X-Debug", Value: "1"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}, 1)

	data, err := yaml.Marshal(rf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)
	for _, want := range []string{
		"info:", "name: Create User", "type: http",
		"http:", "method: POST", "url: https://api/users",
		"headers:", "name: X-Debug", "disabled: true",
		"body:", "type: json", "data:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("marshalled request missing %q:\n%s", want, out)
		}
	}
}

func clearIDs(c *model.Collection) {
	c.ID = ""
	for i := range c.Requests {
		c.Requests[i].ID = ""
	}
	for i := range c.Folders {
		clearFolderIDs(&c.Folders[i])
	}
	for i := range c.Environments {
		c.Environments[i].ID = ""
	}
}

func clearFolderIDs(f *model.Folder) {
	f.ID = ""
	for i := range f.Requests {
		f.Requests[i].ID = ""
	}
	for i := range f.Folders {
		clearFolderIDs(&f.Folders[i])
	}
}
