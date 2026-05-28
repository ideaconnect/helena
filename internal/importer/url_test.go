package importer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

func TestFromURLFetchesAndParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(oas3Sample))
	}))
	defer srv.Close()

	c, err := FromURL(srv.URL, model.Settings{TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	if c.Name != "Sample API" {
		t.Errorf("collection name = %q, want Sample API", c.Name)
	}
	if len(c.Folders) == 0 && len(c.Requests) == 0 {
		t.Errorf("no items imported")
	}
}

func TestFromURLFetchesWSDL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(wsdlSample))
	}))
	defer srv.Close()

	c, err := FromURL(srv.URL, model.Settings{TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	if c.Name != "Calc" {
		t.Errorf("collection name = %q, want Calc", c.Name)
	}
}

func TestFromURLNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FromURL(srv.URL, model.Settings{TimeoutSeconds: 5})
	if err == nil {
		t.Fatal("expected non-2xx response to error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status code", err)
	}
}

func TestFromURLNetworkErrorPropagates(t *testing.T) {
	// Closed server → connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close()

	_, err := FromURL(addr, model.Settings{TimeoutSeconds: 2})
	if err == nil {
		t.Fatal("expected network error")
	}
}
