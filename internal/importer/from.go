package importer

import "github.com/idct/helena/internal/model"

// From parses an API description and returns the resulting Helena collection.
// XML input is treated as WSDL; everything else as OpenAPI 3 / Swagger 2.
func From(data []byte) (model.Collection, error) {
	if looksLikeXML(data) {
		return FromWSDL(data)
	}
	return FromOpenAPI(data)
}

func looksLikeXML(data []byte) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}
