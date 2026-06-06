package importer

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/idct/helena/internal/model"
)

// maxSpecBytes bounds how much of a fetched spec is read into memory; a real
// OpenAPI/WSDL spec is far smaller, and an unbounded read of an untrusted URL
// could OOM the app.
const maxSpecBytes = 64 << 20 // 64 MiB

// FromURL fetches url and parses the response body as an OpenAPI / Swagger /
// WSDL spec via From. TLS verification is skipped when InsecureSkipVerify is
// true; the fetch honors TimeoutSeconds (0 = no timeout, matching the rest of
// the app's settings semantics). Non-2xx responses are returned as errors.
func FromURL(url string, settings model.Settings) (model.Collection, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: settings.InsecureSkipVerify},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(settings.TimeoutSeconds) * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return model.Collection{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return model.Collection{}, fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes))
	if err != nil {
		return model.Collection{}, err
	}
	return From(body)
}
