package ui

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestAuthTabLoadsAWSV4 verifies an AWS SigV4 request selects the type and
// populates the access key + region (#76).
func TestAuthTabLoadsAWSV4(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthAWSV4, AWSV4: &model.AWSV4Auth{AccessKeyID: "AKID", Region: "eu-west-1"}}}
	m.loadRequest(req, "0/r0")
	if got := m.authType.Selected; got != "AWS Signature v4" {
		t.Errorf("authType.Selected = %q, want AWS Signature v4", got)
	}
	if got := m.authAWSV4AccessKey.Text; got != "AKID" {
		t.Errorf("access key = %q, want AKID", got)
	}
	if got := m.authAWSV4Region.Text; got != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1", got)
	}
}

// TestAuthTabAWSV4WriteBack verifies typing into the AWS SigV4 fields writes
// back into the request's Auth.AWSV4 struct (lazily allocated).
func TestAuthTabAWSV4WriteBack(t *testing.T) {
	m := newAuthUI(t)
	req := &model.Request{Auth: model.Auth{Type: model.AuthAWSV4}}
	m.loadRequest(req, "0/r0")

	m.authAWSV4AccessKey.OnChanged("AKID")
	m.authAWSV4SecretKey.OnChanged("secret")
	m.authAWSV4Region.OnChanged("us-west-2")
	m.authAWSV4Service.OnChanged("s3")
	m.authAWSV4SessionToken.OnChanged("tok")

	v := req.Auth.AWSV4
	if v == nil || v.AccessKeyID != "AKID" || v.SecretAccessKey != "secret" ||
		v.Region != "us-west-2" || v.Service != "s3" || v.SessionToken != "tok" {
		t.Errorf("AWSV4 = %+v, want AKID/secret/us-west-2/s3/tok", v)
	}
}
