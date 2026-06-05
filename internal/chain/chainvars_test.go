package chain

import (
	"net/http"
	"testing"
)

// TestVarLookup verifies the {{chain.<alias>...}} template navigator across the
// JSON / headers / status / body / request surfaces, plus its not-found cases.
func TestVarLookup(t *testing.T) {
	views := map[string]View{
		"login": {
			Request: RequestView{Method: "POST", URL: "https://api/login", Body: []byte("user=x")},
			Response: ResponseView{
				StatusCode: 200,
				Status:     "200 OK",
				Headers:    http.Header{"X-Token": {"hdr-val"}},
				Body: []byte(`{"token":"abc","exp":3600,"ok":true,"nil":null,` +
					`"nested":{"id":"x1"},"items":[{"id":"a"},{"id":"b"}]}`),
			},
		},
		"bad": {Response: ResponseView{Body: []byte("{not json")}},
	}
	lookup := VarLookup(views)

	ok := []struct{ name, want string }{
		{"chain.login.response.json.token", "abc"},
		{"chain.login.response.json.exp", "3600"}, // json.Number keeps exact text
		{"chain.login.response.json.ok", "true"},  // bool
		{"chain.login.response.json.nil", ""},     // explicit null → empty
		{"chain.login.response.json.nested.id", "x1"},
		{"chain.login.response.json.items.1.id", "b"}, // array index
		{"chain.login.response.headers.X-Token", "hdr-val"},
		{"chain.login.response.headers.x-token", "hdr-val"}, // case-insensitive
		{"chain.login.response.status", "200"},
		{"chain.login.response.statusText", "200 OK"},
		{"chain.login.response.body", `{"token":"abc","exp":3600,"ok":true,"nil":null,"nested":{"id":"x1"},"items":[{"id":"a"},{"id":"b"}]}`},
		{"chain.login.response.text", `{"token":"abc","exp":3600,"ok":true,"nil":null,"nested":{"id":"x1"},"items":[{"id":"a"},{"id":"b"}]}`},
		{"chain.login.request.method", "POST"},
		{"chain.login.request.url", "https://api/login"},
		{"chain.login.request.body", "user=x"},
	}
	for _, c := range ok {
		if got, found := lookup(c.name); !found || got != c.want {
			t.Errorf("lookup(%q) = %q,%v; want %q,true", c.name, got, found, c.want)
		}
	}

	notFound := []string{
		"chain.login.response.json.missing", // unknown key
		"chain.login.response.json.items.5", // out of range
		"chain.login.response.json.nested",  // object, not a scalar
		"chain.login.response.json",         // root object, not a scalar
		"chain.login.response.headers.Absent",
		"chain.login.response.headers",      // no header name
		"chain.login.response.foo",          // unknown response field
		"chain.login.request.foo",           // unknown request field
		"chain.login",                       // no section
		"chain.unknown.response.json.token", // unknown alias
		"chain.bad.response.json.x",         // malformed JSON body
		"notchain.x",                        // missing prefix
		"base_url",                          // ordinary var
	}
	for _, n := range notFound {
		if got, found := lookup(n); found {
			t.Errorf("lookup(%q) = %q,true; want not-found", n, got)
		}
	}
}
