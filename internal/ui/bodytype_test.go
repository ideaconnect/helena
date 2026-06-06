package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// ctValue returns the value of the first Content-Type header (case-insensitive),
// and whether such a header exists.
func ctValue(headers []model.KeyValue) (string, bool) {
	for _, h := range headers {
		if h.Key == "Content-Type" {
			return h.Value, true
		}
	}
	return "", false
}

// TestApplyImpliedContentType pins the body-Type → Content-Type sync: a header is
// added when none is set, kept in step while it still holds an implied value, and
// left untouched once the user has customised it.
func TestApplyImpliedContentType(t *testing.T) {
	json := "application/json"
	xml := "application/xml"

	t.Run("adds when absent and type implies one", func(t *testing.T) {
		out, changed := applyImpliedContentType(nil, model.BodyNone, model.BodyJSON)
		if !changed {
			t.Fatal("expected change")
		}
		if v, ok := ctValue(out); !ok || v != json {
			t.Errorf("Content-Type = %q (present=%v), want %q", v, ok, json)
		}
		if len(out) != 1 || !out[0].Enabled {
			t.Errorf("added header = %+v, want one enabled row", out)
		}
	})

	t.Run("no header added when new type implies none", func(t *testing.T) {
		if out, changed := applyImpliedContentType(nil, model.BodyNone, model.BodyNone); changed || len(out) != 0 {
			t.Errorf("None→None changed=%v headers=%v, want no-op", changed, out)
		}
		if out, changed := applyImpliedContentType(nil, model.BodyNone, model.BodyMultipart); changed || len(out) != 0 {
			t.Errorf("None→Multipart changed=%v headers=%v, want no-op (boundary set on send)", changed, out)
		}
	})

	t.Run("updates an auto-managed value to the new type", func(t *testing.T) {
		in := []model.KeyValue{{Enabled: true, Key: "Content-Type", Value: json}}
		out, changed := applyImpliedContentType(in, model.BodyJSON, model.BodyXML)
		if !changed {
			t.Fatal("expected change")
		}
		if v, _ := ctValue(out); v != xml {
			t.Errorf("Content-Type = %q, want %q", v, xml)
		}
	})

	t.Run("removes an auto-managed value when new type implies none", func(t *testing.T) {
		in := []model.KeyValue{{Enabled: true, Key: "Content-Type", Value: json}}
		out, changed := applyImpliedContentType(in, model.BodyJSON, model.BodyMultipart)
		if !changed {
			t.Fatal("expected change")
		}
		if _, ok := ctValue(out); ok {
			t.Errorf("Content-Type still present after switch to multipart: %v", out)
		}
	})

	t.Run("leaves a custom value untouched", func(t *testing.T) {
		custom := "application/vnd.api+json"
		in := []model.KeyValue{{Enabled: true, Key: "Content-Type", Value: custom}}
		out, changed := applyImpliedContentType(in, model.BodyJSON, model.BodyXML)
		if changed {
			t.Error("custom Content-Type should not change")
		}
		if v, _ := ctValue(out); v != custom {
			t.Errorf("Content-Type = %q, want unchanged %q", v, custom)
		}
	})

	t.Run("matches the header key case-insensitively and preserves siblings", func(t *testing.T) {
		in := []model.KeyValue{
			{Enabled: true, Key: "Accept", Value: "*/*"},
			{Enabled: true, Key: "content-type", Value: json},
		}
		out, changed := applyImpliedContentType(in, model.BodyJSON, model.BodyXML)
		if !changed {
			t.Fatal("expected change")
		}
		if len(out) != 2 || out[0].Key != "Accept" {
			t.Errorf("siblings not preserved: %+v", out)
		}
		if out[1].Value != xml {
			t.Errorf("Content-Type = %q, want %q", out[1].Value, xml)
		}
	})
}
