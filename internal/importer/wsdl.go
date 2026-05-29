package importer

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/idct/helena/internal/model"
)

// FromWSDL parses a WSDL 1.1 (SOAP) document and produces a Helena collection
// with one request per operation, pre-configured with the port's endpoint URL,
// a SOAPAction header, and a placeholder SOAP envelope body. Users fill in the
// operation parameters themselves — this importer is intentionally thin: it
// enumerates operations to give you a starting point, not a full schema-aware
// payload.
func FromWSDL(data []byte) (model.Collection, error) {
	var def wsdlDefinitions
	if err := xml.Unmarshal(data, &def); err != nil {
		return model.Collection{}, fmt.Errorf("parse wsdl: %w", err)
	}

	name := stringOr(firstServiceName(def.Services), "Imported SOAP service")
	c := model.Collection{ID: model.NewID(), Name: name}

	bindings := make(map[string]wsdlBinding, len(def.Bindings))
	for _, b := range def.Bindings {
		bindings[b.Name] = b
	}

	seen := map[string]bool{}
	for _, svc := range def.Services {
		for _, port := range svc.Ports {
			b, ok := bindings[localXMLName(port.Binding)]
			if !ok {
				continue
			}
			for _, op := range b.Operations {
				key := svc.Name + "." + port.Name + "." + op.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				c.Requests = append(c.Requests, buildSOAPRequest(
					op.Name,
					port.SoapAddress.Location,
					op.SoapOperation.SoapAction,
					def.TargetNamespace,
				))
			}
		}
	}

	if len(c.Requests) == 0 {
		return model.Collection{}, fmt.Errorf("no SOAP operations found in WSDL")
	}
	return c, nil
}

func firstServiceName(svcs []wsdlService) string {
	if len(svcs) > 0 {
		return svcs[0].Name
	}
	return ""
}

// localXMLName strips an optional XML namespace prefix ("tns:Foo" -> "Foo").
func localXMLName(qname string) string {
	if i := strings.IndexByte(qname, ':'); i >= 0 {
		return qname[i+1:]
	}
	return qname
}

func buildSOAPRequest(opName, endpoint, soapAction, targetNS string) model.Request {
	var env strings.Builder
	env.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	env.WriteString(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"`)
	if targetNS != "" {
		fmt.Fprintf(&env, ` xmlns:tns=%q`, targetNS)
	}
	env.WriteString(">\n")
	env.WriteString("  <soapenv:Header/>\n")
	env.WriteString("  <soapenv:Body>\n")
	prefix := ""
	if targetNS != "" {
		prefix = "tns:"
	}
	fmt.Fprintf(&env, "    <%s%s>\n", prefix, opName)
	env.WriteString("      <!-- Fill in operation parameters here. -->\n")
	fmt.Fprintf(&env, "    </%s%s>\n", prefix, opName)
	env.WriteString("  </soapenv:Body>\n")
	env.WriteString("</soapenv:Envelope>\n")

	headers := []model.KeyValue{
		{Enabled: true, Key: "Content-Type", Value: "text/xml; charset=utf-8"},
	}
	if soapAction != "" {
		headers = append(headers, model.KeyValue{
			Enabled: true, Key: "SOAPAction", Value: soapAction,
		})
	}

	return model.Request{
		ID:      model.NewID(),
		Name:    opName,
		Method:  model.POST,
		URL:     endpoint,
		Headers: headers,
		Body:    model.Body{Type: model.BodyXML, Content: env.String()},
	}
}

// --- WSDL parse types (captures just the bits Helena needs) ---

// wsdlDefinitions is the root <definitions> element. We capture only the
// targetNamespace and the service / binding lists; ports, messages and types
// are ignored because Helena emits a placeholder envelope rather than a fully
// typed payload.
type wsdlDefinitions struct {
	XMLName         xml.Name      `xml:"definitions"`
	TargetNamespace string        `xml:"targetNamespace,attr"`
	Services        []wsdlService `xml:"service"`
	Bindings        []wsdlBinding `xml:"binding"`
}

// wsdlService groups one or more ports under a service name; the service name
// becomes the imported collection's display name.
type wsdlService struct {
	Name  string     `xml:"name,attr"`
	Ports []wsdlPort `xml:"port"`
}

// wsdlPort ties a binding to a concrete SOAP endpoint URL.
type wsdlPort struct {
	Name        string          `xml:"name,attr"`
	Binding     string          `xml:"binding,attr"`
	SoapAddress wsdlSoapAddress `xml:"address"`
}

// wsdlSoapAddress carries the endpoint URL the SOAP request will be POSTed to.
type wsdlSoapAddress struct {
	Location string `xml:"location,attr"`
}

// wsdlBinding lists the SOAP operations exposed by a binding; we use Name to
// match against wsdlPort.Binding (after stripping any namespace prefix).
type wsdlBinding struct {
	Name       string                 `xml:"name,attr"`
	Operations []wsdlBindingOperation `xml:"operation"`
}

// wsdlBindingOperation pairs a WSDL operation name with its SOAP-specific
// metadata (SOAPAction).
type wsdlBindingOperation struct {
	Name          string            `xml:"name,attr"`
	SoapOperation wsdlSoapOperation `xml:"operation"`
}

// wsdlSoapOperation captures soapAction; an empty value means no SOAPAction
// header should be emitted for this operation.
type wsdlSoapOperation struct {
	SoapAction string `xml:"soapAction,attr"`
}
