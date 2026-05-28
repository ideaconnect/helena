// Package examples_test contains a smoke test that loads the bundled sample
// collections through the real storage layer so a format drift can't ship
// alongside undetected breakage of the on-disk examples.
package examples_test

import (
	"testing"

	"github.com/idct/helena/internal/storage"
)

func TestHttpbinSampleLoads(t *testing.T) {
	c, err := storage.Load("httpbin")
	if err != nil {
		t.Fatalf("Load(httpbin): %v", err)
	}
	if c.Name != "httpbin sample" {
		t.Errorf("name = %q, want %q", c.Name, "httpbin sample")
	}
	if len(c.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(c.Requests))
	}
	if len(c.Environments) != 1 {
		t.Fatalf("environments = %d, want 1", len(c.Environments))
	}

	gotMethods := map[string]string{}
	for _, r := range c.Requests {
		gotMethods[r.Name] = string(r.Method)
	}
	if gotMethods["GET /anything"] != "GET" || gotMethods["POST /anything"] != "POST" {
		t.Errorf("methods mismatch: %+v", gotMethods)
	}

	env := c.Environments[0]
	if env.Name != "default" {
		t.Errorf("env name = %q", env.Name)
	}
	found := false
	for _, v := range env.Variables {
		if v.Key == "base_url" && v.Value == "https://httpbin.org" {
			found = true
		}
	}
	if !found {
		t.Errorf("base_url var not found in env: %+v", env.Variables)
	}
}
