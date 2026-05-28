package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
)

func TestRequestEditPersists(t *testing.T) {
	dir := writeCollectionWithEnv(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	// Edit the sample collection's only root request via the tree pointer.
	r, ok := s.Tree().Request("0/r0")
	if !ok {
		t.Fatal("Request not found at 0/r0")
	}
	r.URL = "https://changed.example/{{base}}/users"
	r.Method = model.POST
	r.Headers = append(r.Headers, model.KeyValue{Enabled: true, Key: "X-Test", Value: "yes"})
	r.Params = append(r.Params, model.KeyValue{Enabled: true, Key: "page", Value: "1"})
	r.Body = model.Body{Type: model.BodyJSON, Content: `{"a":1}`}

	if err := s.SaveActiveCollection(); err != nil {
		t.Fatalf("SaveActiveCollection: %v", err)
	}

	// Fresh session must see all edits.
	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	r2, ok := s2.Tree().Request("0/r0")
	if !ok {
		t.Fatal("Request not found after reload")
	}
	if r2.URL != "https://changed.example/{{base}}/users" {
		t.Errorf("URL = %q", r2.URL)
	}
	if r2.Method != model.POST {
		t.Errorf("Method = %q, want POST", r2.Method)
	}
	if len(r2.Headers) != 1 || r2.Headers[0].Key != "X-Test" || r2.Headers[0].Value != "yes" || !r2.Headers[0].Enabled {
		t.Errorf("Headers = %+v", r2.Headers)
	}
	if len(r2.Params) != 1 || r2.Params[0].Key != "page" || r2.Params[0].Value != "1" {
		t.Errorf("Params = %+v", r2.Params)
	}
	if r2.Body.Type != model.BodyJSON || r2.Body.Content != `{"a":1}` {
		t.Errorf("Body = %+v", r2.Body)
	}
}
