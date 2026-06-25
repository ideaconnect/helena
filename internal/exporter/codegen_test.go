package exporter

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/vars"
)

// jsonPOST is a representative request: a JSON POST with one explicit header
// (httpclient.Build adds Content-Type: application/json, sorted first).
func jsonPOST() model.Request {
	return model.Request{
		Method:  model.POST,
		URL:     "https://api.example/users",
		Headers: []model.KeyValue{{Enabled: true, Key: "X-Token", Value: "abc"}},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
}

// TestToFetch pins the JavaScript fetch() output for a JSON POST.
func TestToFetch(t *testing.T) {
	got, err := ToFetch(jsonPOST(), nil, model.Settings{})
	if err != nil {
		t.Fatalf("ToFetch: %v", err)
	}
	want := `fetch("https://api.example/users", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Token": "abc"
  },
  body: "{\"a\":1}",
})
  .then((r) => r.text())
  .then(console.log);`
	if got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestToPython pins the Python requests output, including the verify /
// allow_redirects / timeout kwargs from settings.
func TestToPython(t *testing.T) {
	got, err := ToPython(jsonPOST(), nil, model.Settings{
		InsecureSkipVerify: true, FollowRedirects: false, TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ToPython: %v", err)
	}
	want := `import requests

response = requests.request(
    "POST",
    "https://api.example/users",
    headers={
        "Content-Type": "application/json",
        "X-Token": "abc",
    },
    data="{\"a\":1}",
    verify=False,
    allow_redirects=False,
    timeout=30,
)
print(response.text)`
	if got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestToGoSimple pins the Go net/http output for a no-body GET with redirect-
// following on, which uses http.DefaultClient (no custom client block).
func TestToGoSimple(t *testing.T) {
	got, err := ToGo(
		model.Request{Method: model.GET, URL: "https://example.com/x"},
		nil, model.Settings{FollowRedirects: true},
	)
	if err != nil {
		t.Fatalf("ToGo: %v", err)
	}
	want := `package main

import (
	"fmt"
	"io"
	"net/http"
)

func main() {
	req, err := http.NewRequest("GET", "https://example.com/x", nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Println(string(out))
}`
	if got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestToGoCustomClient pins the Go output for a JSON POST with insecure TLS,
// which emits the body, headers, and a configured http.Client.
func TestToGoCustomClient(t *testing.T) {
	got, err := ToGo(jsonPOST(), nil, model.Settings{FollowRedirects: true, InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("ToGo: %v", err)
	}
	want := `package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	body := strings.NewReader("{\"a\":1}")
	req, err := http.NewRequest("POST", "https://api.example/users", body)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Token", "abc")
	client := &http.Client{}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Println(string(out))
}`
	if got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestCodegenResolvesVars verifies generators resolve {{vars}} (shared
// httpclient.Build path) — the URL var is substituted in the output.
func TestCodegenResolvesVars(t *testing.T) {
	r := model.Request{Method: model.GET, URL: "{{base}}/ping"}
	res := vars.New(map[string]string{"base": "https://h.test"})
	for name, fn := range map[string]func() (string, error){
		"fetch":  func() (string, error) { return ToFetch(r, res, model.Settings{}) },
		"python": func() (string, error) { return ToPython(r, res, model.Settings{}) },
		"go":     func() (string, error) { return ToGo(r, res, model.Settings{FollowRedirects: true}) },
	} {
		got, err := fn()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(got, "https://h.test/ping") {
			t.Errorf("%s did not resolve {{base}}:\n%s", name, got)
		}
	}
}
