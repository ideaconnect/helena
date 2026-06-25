package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// awsVanillaAuth is the AWS SigV4 test-suite "get-vanilla" fixture: AccessKey
// AKIDEXAMPLE, service/us-east-1, at 20150830T123600Z.
func awsVanillaAuth() *model.AWSV4Auth {
	return &model.AWSV4Auth{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
		Service:         "service",
	}
}

// TestAWSSigV4GetVanilla pins the signer against the AWS SigV4 "get-vanilla"
// vector. The expected signature was computed by a clean-room Python SigV4
// implementation; its string-to-sign hashes the canonical request to
// bb579772317eb040ac9ed261061d46c1f17a8133879d6129b6e1c25292927e63 — AWS's
// published get-vanilla canonical-request hash — anchoring the value to AWS's
// own documented intermediate (the final signature is a deterministic HMAC of
// that string-to-sign under the four-step signing key).
func TestAWSSigV4GetVanilla(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/", nil)
	req.Host = "example.amazonaws.com"
	req.Header.Set("X-Amz-Date", "20150830T123600Z")

	got := awsSigV4Header(req, awsVanillaAuth(), "20150830T123600Z", emptyPayloadHash)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, " +
		"SignedHeaders=host;x-amz-date, " +
		"Signature=ea21d6f05e96a897f6000a1a293f0a5bf0f92a00343409e820dce329ca6365ea"
	if got != want {
		t.Errorf("awsSigV4Header =\n  %q\nwant\n  %q", got, want)
	}
}

// TestAWSSigV4Query verifies query-string params are canonicalized (encoded +
// sorted) into the signature. Same fixture as get-vanilla with two params that
// sort by key; signature independently recomputed via the Python reference.
func TestAWSSigV4Query(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/?Param2=value2&Param1=value1", nil)
	req.Host = "example.amazonaws.com"
	req.Header.Set("X-Amz-Date", "20150830T123600Z")
	got := awsSigV4Header(req, awsVanillaAuth(), "20150830T123600Z", emptyPayloadHash)
	want := "Signature=8d42a939124c7caa12286d7c29afe0cd5356d0897447891c374aba0aceb3b785"
	if !strings.Contains(got, want) {
		t.Errorf("awsSigV4Header = %q, want substring %q", got, want)
	}
}

// TestApplyAWSV4 verifies Apply sets X-Amz-Date + an AWS4 Authorization header,
// folds a session token into the signed headers, resolves {{vars}}, and that a
// user-set Authorization wins.
func TestApplyAWSV4(t *testing.T) {
	a := model.Auth{Type: model.AuthAWSV4, AWSV4: &model.AWSV4Auth{
		AccessKeyID: "{{key}}", SecretAccessKey: "secret", Region: "eu-west-1",
		Service: "s3", SessionToken: "tok",
	}}
	a = ResolveValues(a, func(s string) string {
		if s == "{{key}}" {
			return "AKID"
		}
		return s
	})

	req, _ := http.NewRequest(http.MethodGet, "https://b.s3.eu-west-1.amazonaws.com/o", nil)
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("X-Amz-Date not set")
	}
	if req.Header.Get("X-Amz-Security-Token") != "tok" {
		t.Errorf("X-Amz-Security-Token = %q, want tok", req.Header.Get("X-Amz-Security-Token"))
	}
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "AWS4-HMAC-SHA256 Credential=AKID/") {
		t.Fatalf("Authorization = %q", h)
	}
	if !strings.Contains(h, "eu-west-1/s3/aws4_request") {
		t.Errorf("scope wrong: %q", h)
	}
	// Session token must be in the signed-header set so it's covered by the sig.
	if !strings.Contains(h, "x-amz-security-token") {
		t.Errorf("session token not signed: %q", h)
	}
}

// TestApplyAWSV4Defaults verifies blank region/service default and a POST body
// is hashed (non-empty payload hash) into the signature.
func TestApplyAWSV4Defaults(t *testing.T) {
	body := "hello=world"
	req, _ := http.NewRequest(http.MethodPost, "https://x.amazonaws.com/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	a := model.Auth{Type: model.AuthAWSV4, AWSV4: &model.AWSV4Auth{AccessKeyID: "AKID", SecretAccessKey: "s"}}
	if err := Apply(context.Background(), req, a, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	h := req.Header.Get("Authorization")
	if !strings.Contains(h, "/us-east-1/service/aws4_request") {
		t.Errorf("defaults not applied: %q", h)
	}
	// content-type is signed for the bodied request.
	if !strings.Contains(h, "content-type") {
		t.Errorf("content-type not signed: %q", h)
	}
}

// TestApplyAWSV4NilAndOverwrite verifies a nil sub-struct is a no-op and an
// existing Authorization header is preserved.
func TestApplyAWSV4NilAndOverwrite(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://x/", nil)
	if err := Apply(context.Background(), req, model.Auth{Type: model.AuthAWSV4}, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" || req.Header.Get("X-Amz-Date") != "" {
		t.Error("nil AWSV4 should set nothing")
	}
	req.Header.Set("Authorization", "preset")
	_ = Apply(context.Background(), req, model.Auth{Type: model.AuthAWSV4, AWSV4: &model.AWSV4Auth{AccessKeyID: "k"}}, nil)
	if req.Header.Get("Authorization") != "preset" {
		t.Error("AWSV4 overwrote an existing Authorization header")
	}
}

// TestAWSURIEncode covers the RFC 3986 encoding helper for unreserved,
// reserved, and slash (kept vs encoded).
func TestAWSURIEncode(t *testing.T) {
	if got := awsURIEncode("a/b c~d", false); got != "a%2Fb%20c~d" {
		t.Errorf("awsURIEncode(keepSlash=false) = %q", got)
	}
	if got := awsURIEncode("a/b c", true); got != "a/b%20c" {
		t.Errorf("awsURIEncode(keepSlash=true) = %q", got)
	}
}
