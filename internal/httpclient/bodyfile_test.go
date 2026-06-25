package httpclient

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestBuildFileBodySendsExactBytes pins #24: a BodyFile request sends the file's
// exact bytes with the chosen Content-Type.
func TestBuildFileBodySendsExactBytes(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0xff, 'h', 'i', 0x00}
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	req, body, err := Build(context.Background(), model.Request{
		Method: model.POST, URL: "https://x.test/upload",
		Body: model.Body{Type: model.BodyFile, FilePath: path, ContentType: "image/png"},
	}, vars.New(nil), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytes.Equal(body, raw) {
		t.Errorf("body = %v, want exact file bytes %v", body, raw)
	}
	if ct := req.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

// TestBuildFileBodyDefaultsContentType verifies an unset ContentType defaults to
// application/octet-stream.
func TestBuildFileBodyDefaultsContentType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.dat")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	req, _, err := Build(context.Background(), model.Request{
		Method: model.PUT, URL: "https://x.test/o",
		Body: model.Body{Type: model.BodyFile, FilePath: path},
	}, vars.New(nil), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ct := req.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}
}

// TestBuildFileBodyUserContentTypeWins verifies a user-set Content-Type header
// overrides the body's advertised type.
func TestBuildFileBodyUserContentTypeWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.dat")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	req, _, err := Build(context.Background(), model.Request{
		Method: model.POST, URL: "https://x.test/o",
		Headers: []model.KeyValue{{Enabled: true, Key: "Content-Type", Value: "text/csv"}},
		Body:    model.Body{Type: model.BodyFile, FilePath: path, ContentType: "image/png"},
	}, vars.New(nil), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ct := req.Header.Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type = %q, want user header text/csv to win", ct)
	}
}

// TestBuildFileBodyEmptyPathNoBody verifies an unset FilePath yields no body
// (the user hasn't picked a file yet) rather than an error.
func TestBuildFileBodyEmptyPathNoBody(t *testing.T) {
	_, body, err := Build(context.Background(), model.Request{
		Method: model.POST, URL: "https://x.test/o",
		Body: model.Body{Type: model.BodyFile},
	}, vars.New(nil), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %v, want empty for no file path", body)
	}
}

// TestBuildFileBodyMissingFileErrors verifies a non-existent file path is a
// build error.
func TestBuildFileBodyMissingFileErrors(t *testing.T) {
	_, _, err := Build(context.Background(), model.Request{
		Method: model.POST, URL: "https://x.test/o",
		Body: model.Body{Type: model.BodyFile, FilePath: filepath.Join(t.TempDir(), "nope.bin")},
	}, vars.New(nil), nil)
	if err == nil {
		t.Error("expected an error for a missing body file")
	}
}
