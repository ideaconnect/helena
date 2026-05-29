package importer

import (
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

const wsdlSample = `<?xml version="1.0"?>
<wsdl:definitions xmlns:wsdl="http://schemas.xmlsoap.org/wsdl/"
                  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
                  xmlns:tns="http://example.com/calc"
                  targetNamespace="http://example.com/calc">
  <wsdl:portType name="CalcPort"/>
  <wsdl:binding name="CalcBinding" type="tns:CalcPort">
    <soap:binding transport="http://schemas.xmlsoap.org/soap/http"/>
    <wsdl:operation name="Add">
      <soap:operation soapAction="http://example.com/calc/Add"/>
    </wsdl:operation>
    <wsdl:operation name="Subtract">
      <soap:operation soapAction=""/>
    </wsdl:operation>
  </wsdl:binding>
  <wsdl:service name="Calc">
    <wsdl:port name="CalcPort" binding="tns:CalcBinding">
      <soap:address location="http://example.com/calc"/>
    </wsdl:port>
  </wsdl:service>
</wsdl:definitions>`

// TestFromWSDL verifies a WSDL 1.1 doc yields one POST request per operation with the port endpoint, SOAPAction header, and a placeholder envelope.
func TestFromWSDL(t *testing.T) {
	c, err := FromWSDL([]byte(wsdlSample))
	if err != nil {
		t.Fatalf("FromWSDL: %v", err)
	}
	if c.Name != "Calc" {
		t.Errorf("collection name = %q, want Calc", c.Name)
	}
	if len(c.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(c.Requests))
	}

	byName := map[string]*model.Request{}
	for i := range c.Requests {
		r := &c.Requests[i]
		byName[r.Name] = r
		if r.Method != model.POST {
			t.Errorf("%s method = %s, want POST", r.Name, r.Method)
		}
		if r.URL != "http://example.com/calc" {
			t.Errorf("%s URL = %q", r.Name, r.URL)
		}
		if r.Body.Type != model.BodyXML {
			t.Errorf("%s body type = %q", r.Name, r.Body.Type)
		}
		if !strings.Contains(r.Body.Content, "<tns:"+r.Name+">") {
			t.Errorf("%s body missing operation element:\n%s", r.Name, r.Body.Content)
		}
		if !hasHeader(r, "Content-Type", "text/xml; charset=utf-8") {
			t.Errorf("%s missing Content-Type header: %+v", r.Name, r.Headers)
		}
	}

	add := byName["Add"]
	if add == nil {
		t.Fatal("Add operation not found")
	}
	if !hasHeader(add, "SOAPAction", "http://example.com/calc/Add") {
		t.Errorf("Add missing SOAPAction: %+v", add.Headers)
	}

	sub := byName["Subtract"]
	if sub == nil {
		t.Fatal("Subtract operation not found")
	}
	// Empty soapAction in the binding means we don't emit the header.
	for _, h := range sub.Headers {
		if h.Key == "SOAPAction" {
			t.Errorf("Subtract should not have SOAPAction header (was empty): %+v", h)
		}
	}
}

// TestFromWSDLNoOperationsErrors verifies an empty WSDL definitions element produces an error rather than an empty collection.
func TestFromWSDLNoOperationsErrors(t *testing.T) {
	_, err := FromWSDL([]byte(`<?xml version="1.0"?><definitions xmlns="http://schemas.xmlsoap.org/wsdl/"/>`))
	if err == nil {
		t.Fatal("expected error for WSDL with no operations")
	}
}

// TestFromDispatcher verifies From routes XML to FromWSDL and JSON/YAML to FromOpenAPI based on the leading-byte sniff.
func TestFromDispatcher(t *testing.T) {
	// XML → WSDL path.
	if c, err := From([]byte(wsdlSample)); err != nil || c.Name != "Calc" {
		t.Errorf("From(wsdl) = %+v, err=%v", c, err)
	}
	// OpenAPI YAML → OpenAPI path.
	if c, err := From([]byte(oas3Sample)); err != nil || c.Name != "Sample API" {
		t.Errorf("From(oas3) = %+v, err=%v", c, err)
	}
	// Swagger 2 → OpenAPI path.
	if c, err := From([]byte(swagger2Sample)); err != nil || c.Name != "Legacy API" {
		t.Errorf("From(swagger2) = %+v, err=%v", c, err)
	}
}

func hasHeader(r *model.Request, key, value string) bool {
	for _, h := range r.Headers {
		if h.Key == key && h.Value == value {
			return true
		}
	}
	return false
}
