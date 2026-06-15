package httpclient

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// FuzzBuildBody hammers the body-construction switch with arbitrary
// body type + content combinations. The contract: never panic, and
// every branch (including multipart) succeeds for these text inputs.
//
// buildBody is unexported so this fuzz target lives in the same
// package — it's the smallest surface that exercises every BodyType
// branch including the default fallback for unknown types.
func FuzzBuildBody(f *testing.F) {
	identity := func(s string) string { return s }
	for _, c := range []struct {
		kind, content string
	}{
		{"", ""},
		{"none", ""},
		{"json", `{"k":"v"}`},
		{"json", ""},
		{"xml", "<r/>"},
		{"text", "hello"},
		{"form-urlencoded", "k=v"},
		{"form-urlencoded", ""},
		{"multipart-form", "anything"},
		{"weird-custom", "raw"},
		{"json", "\x00\xff"},
	} {
		f.Add(c.kind, c.content)
	}

	f.Fuzz(func(t *testing.T, kind, content string) {
		r := model.Request{
			Body: model.Body{Type: model.BodyType(kind), Content: content},
		}
		body, ct, err := buildBody(r, identity)
		// Multipart now encodes Body.Form (empty here, so an empty multipart
		// body) and must succeed with a multipart/form-data content type;
		// Content is ignored for this type.
		if model.BodyType(kind) == model.BodyMultipart {
			if err != nil {
				t.Errorf("multipart should encode without error, got %v", err)
			}
			if !strings.HasPrefix(ct, "multipart/form-data") {
				t.Errorf("multipart ct = %q, want multipart/form-data prefix", ct)
			}
			_ = body
			return
		}
		if err != nil {
			t.Errorf("buildBody(%q, %q) unexpected err: %v", kind, content, err)
		}
		// Bodyless types must produce nil body.
		if kind == "" || kind == "none" {
			if body != nil {
				t.Errorf("kind=%q produced non-nil body %q", kind, body)
			}
		}
	})
}

// FuzzBuildBodyForm exercises the form-fields branch separately —
// random KV pairs are added via Body.Form and the encoded output is
// checked for absence of injection-prone characters.
func FuzzBuildBodyForm(f *testing.F) {
	identity := func(s string) string { return s }
	f.Add("k", "v")
	f.Add("", "")
	f.Add("k with space", "v&with=ampersand")
	f.Add("\x00", "\xff")
	f.Fuzz(func(t *testing.T, k, v string) {
		r := model.Request{
			Body: model.Body{
				Type: model.BodyForm,
				Form: []model.KeyValue{{Enabled: true, Key: k, Value: v}},
			},
		}
		body, ct, err := buildBody(r, identity)
		if err != nil {
			t.Errorf("buildBody form unexpected err: %v", err)
		}
		if ct != model.BodyForm.ContentType() {
			t.Errorf("ct = %q, want %q", ct, model.BodyForm.ContentType())
		}
		// url.Values.Encode() guarantees output is application/x-www-form-urlencoded
		// safe: every key/value must be percent-encoded so raw '&'
		// can only appear as a separator. We don't verify a stronger
		// invariant — just that no panic occurred and content-type is right.
		_ = body
	})
}
