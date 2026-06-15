package httpclient

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestBuildMultipartBody verifies multipart/form-data encoding of Body.Form:
// enabled text fields are written, disabled ones omitted, {{vars}} resolved,
// and the Content-Type carries the generated boundary (regression for #22 —
// multipart previously hard-errored).
func TestBuildMultipartBody(t *testing.T) {
	req, body, err := Build(context.Background(), model.Request{
		Method: model.POST,
		URL:    "https://x.test/upload",
		Body: model.Body{Type: model.BodyMultipart, Form: []model.KeyValue{
			{Enabled: true, Key: "name", Value: "helena"},
			{Enabled: true, Key: "lang", Value: "{{lang}}"},
			{Enabled: false, Key: "skip", Value: "no"},
		}},
	}, vars.New(map[string]string{"lang": "go"}), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/form-data with boundary", ct)
	}
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("ParseMediaType: %v", err)
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	got := map[string]string{}
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		b, _ := io.ReadAll(part)
		got[part.FormName()] = string(b)
	}
	if got["name"] != "helena" {
		t.Errorf("name part = %q, want helena", got["name"])
	}
	if got["lang"] != "go" {
		t.Errorf("lang part = %q, want go ({{lang}} resolved)", got["lang"])
	}
	if _, ok := got["skip"]; ok {
		t.Errorf("disabled field was encoded: %v", got)
	}
}
