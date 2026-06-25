package ui

import (
	"sort"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// TestRequestTemplateStringsFindsPromptVars verifies the Send-time prompt-var
// scan (#86) collects {{?Name}} markers from the URL, body, enabled
// header/param/form values, and auth fields — and skips disabled rows.
func TestRequestTemplateStringsFindsPromptVars(t *testing.T) {
	r := model.Request{
		URL: "https://{{host}}/{{?Env}}",
		Headers: []model.KeyValue{
			{Enabled: true, Key: "Authorization", Value: "Bearer {{?Token}}"},
			{Enabled: false, Key: "X-Off", Value: "{{?Disabled}}"},
		},
		Params: []model.KeyValue{{Enabled: true, Key: "q", Value: "{{?Query}}"}},
		Body:   model.Body{Type: model.BodyJSON, Content: `{"k":"{{?Body}}"}`},
		Auth:   model.Auth{Type: model.AuthBasic, Basic: &model.BasicAuth{Username: "{{?User}}", Password: "static"}},
	}
	got := vars.PromptVars(requestTemplateStrings(r)...)
	sort.Strings(got)
	want := []string{"?Body", "?Env", "?Query", "?Token", "?User"}
	if len(got) != len(want) {
		t.Fatalf("prompt keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prompt keys = %v, want %v (note: ?Disabled must be excluded)", got, want)
			break
		}
	}
}

// TestRequestTemplateStringsNoPrompts verifies a request without {{?...}} yields
// no prompt keys (so Send proceeds without a dialog).
func TestRequestTemplateStringsNoPrompts(t *testing.T) {
	r := model.Request{URL: "https://{{host}}/path", Body: model.Body{Content: `{"a":1}`}}
	if got := vars.PromptVars(requestTemplateStrings(r)...); got != nil {
		t.Errorf("prompt keys = %v, want nil", got)
	}
}
