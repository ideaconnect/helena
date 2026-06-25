package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestMethodValid verifies that Method.Valid accepts a known method and rejects an unknown one.
func TestMethodValid(t *testing.T) {
	if !GET.Valid() {
		t.Errorf("GET should be valid")
	}
	if Method("FOO").Valid() {
		t.Errorf("FOO should not be valid")
	}
}

// TestBodyTypeContentType verifies that each BodyType maps to its expected Content-Type header value.
func TestBodyTypeContentType(t *testing.T) {
	cases := map[BodyType]string{
		BodyJSON:      "application/json",
		BodyXML:       "application/xml",
		BodyText:      "text/plain",
		BodyForm:      "application/x-www-form-urlencoded",
		BodyNone:      "",
		BodyMultipart: "",
		BodyFile:      "",                 // dynamic; the chosen type is applied in httpclient.buildBody (#24)
		BodyGraphQL:   "application/json", // GraphQL POSTs a JSON envelope (#70)
	}
	for bt, want := range cases {
		if got := bt.ContentType(); got != want {
			t.Errorf("%s.ContentType() = %q, want %q", bt, got, want)
		}
	}
}

// TestEnabledPairs verifies that EnabledPairs keeps only enabled entries while preserving order.
func TestEnabledPairs(t *testing.T) {
	in := []KeyValue{
		{Enabled: true, Key: "a"},
		{Enabled: false, Key: "b"},
		{Enabled: true, Key: "c"},
	}
	got := EnabledPairs(in)
	if len(got) != 2 || got[0].Key != "a" || got[1].Key != "c" {
		t.Errorf("EnabledPairs = %+v, want a and c", got)
	}
}

// TestNewIDUnique verifies that NewID returns 32-hex-character IDs that don't collide between calls.
func TestNewIDUnique(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 32 {
		t.Errorf("NewID length = %d, want 32", len(a))
	}
	if a == b {
		t.Errorf("NewID returned duplicate %q", a)
	}
}

// TestCollectionJSONRoundTrip verifies that a Collection survives JSON marshal/unmarshal with full fidelity.
func TestCollectionJSONRoundTrip(t *testing.T) {
	orig := Collection{
		ID:   NewID(),
		Name: "Demo",
		Requests: []Request{{
			ID:      "r1",
			Name:    "Get user",
			Method:  GET,
			URL:     "https://{{host}}/users/{{id}}",
			Headers: []KeyValue{{Enabled: true, Key: "Accept", Value: "application/json"}},
			Params:  []KeyValue{{Enabled: true, Key: "verbose", Value: "true"}},
			Body:    Body{Type: BodyJSON, Content: `{"x":1}`},
		}},
		Environments: []Environment{{
			ID:        "e1",
			Name:      "Local",
			Variables: []Variable{{Enabled: true, Key: "host", Value: "localhost"}},
		}},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Collection
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n orig=%+v\n got =%+v", orig, got)
	}
}

// TestBodyTypeValid verifies that every documented BodyType is
// accepted and an unknown value is rejected. Mirrors TestMethodValid
// for the other discriminator used by the body editor.
func TestBodyTypeValid(t *testing.T) {
	for _, bt := range BodyTypes {
		if !bt.Valid() {
			t.Errorf("BodyType(%q).Valid() = false, want true", bt)
		}
	}
	if BodyType("nonsense").Valid() {
		t.Errorf("nonsense BodyType should not be valid")
	}
}

// TestDefaultSettings verifies the documented defaults: TLS verify
// enabled, CORS advisory on, follow redirects, 30s timeout, system
// theme. These are the values new Helena installs land on, so a
// regression here changes first-run behavior.
func TestDefaultSettings(t *testing.T) {
	got := DefaultSettings()
	if got.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should default to false")
	}
	if !got.CORSWarning {
		t.Error("CORSWarning should default to true")
	}
	if !got.FollowRedirects {
		t.Error("FollowRedirects should default to true")
	}
	if got.TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d, want 30", got.TimeoutSeconds)
	}
	if got.Theme != ThemeSystem {
		t.Errorf("Theme = %q, want %q", got.Theme, ThemeSystem)
	}
}

// TestScriptsIsEmpty verifies that whitespace-only hook bodies are
// treated as empty so the UI Send pipeline can skip the scripting
// runtime entirely.
func TestScriptsIsEmpty(t *testing.T) {
	cases := []struct {
		name string
		s    Scripts
		want bool
	}{
		{"both empty", Scripts{}, true},
		{"whitespace only", Scripts{PreRequest: "  \n\t  "}, true},
		{"pre set", Scripts{PreRequest: "x;"}, false},
		{"post set", Scripts{PostResponse: "y;"}, false},
		{"both set", Scripts{PreRequest: "x;", PostResponse: "y;"}, false},
	}
	for _, c := range cases {
		if got := c.s.IsEmpty(); got != c.want {
			t.Errorf("%s: IsEmpty = %v, want %v", c.name, got, c.want)
		}
	}
}
