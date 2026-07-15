// Package pathparam substitutes single-brace {name} path parameters in a
// request URL. It is the counterpart to internal/vars (which resolves
// double-brace {{variable}} templates): the two syntaxes coexist in one URL —
// "{{base_url}}/drs/bag/{bagId}" — and this package always steps over {{...}}
// spans so a template's inner name is never mistaken for a path parameter.
//
// The package is intentionally free of the model / httpclient / ui packages so
// both the request-build path (which fills tokens at send time) and the editor
// (which lists tokens for the Path tab) can share it.
package pathparam
