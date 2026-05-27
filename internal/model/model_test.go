package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMethodValid(t *testing.T) {
	if !GET.Valid() {
		t.Errorf("GET should be valid")
	}
	if Method("FOO").Valid() {
		t.Errorf("FOO should not be valid")
	}
}

func TestBodyTypeContentType(t *testing.T) {
	cases := map[BodyType]string{
		BodyJSON:      "application/json",
		BodyXML:       "application/xml",
		BodyText:      "text/plain",
		BodyForm:      "application/x-www-form-urlencoded",
		BodyNone:      "",
		BodyMultipart: "",
	}
	for bt, want := range cases {
		if got := bt.ContentType(); got != want {
			t.Errorf("%s.ContentType() = %q, want %q", bt, got, want)
		}
	}
}

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

func TestNewIDUnique(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 32 {
		t.Errorf("NewID length = %d, want 32", len(a))
	}
	if a == b {
		t.Errorf("NewID returned duplicate %q", a)
	}
}

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
