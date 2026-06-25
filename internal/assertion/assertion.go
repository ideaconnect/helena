// Package assertion evaluates declarative (no-code) response checks (#88): a
// list of model.Assertion rows is run against a Send's response and turned into
// pass/fail Results that share the test()/expect() results surface.
package assertion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/idct/helena/internal/model"
)

// Operators supported by Evaluate.
const (
	OpEquals      = "equals"
	OpNotEquals   = "notEquals"
	OpContains    = "contains"
	OpNotContains = "notContains"
	OpMatches     = "matches" // regular expression
	OpGreater     = "greaterThan"
	OpLess        = "lessThan"
	OpExists      = "exists"
	OpNotExists   = "notExists"
)

// Operators lists the supported operators in display order (for the UI picker).
var Operators = []string{
	OpEquals, OpNotEquals, OpContains, OpNotContains, OpMatches, OpGreater, OpLess, OpExists, OpNotExists,
}

// Result is one assertion outcome, shaped like scripting.TestResult so the UI
// can render both through one formatter.
type Result struct {
	Name   string
	Passed bool
	Error  string
}

// Evaluate runs every enabled assertion against the response and returns one
// Result each (disabled rows are skipped). It never errors — a malformed
// source/regex becomes a failing Result.
func Evaluate(assertions []model.Assertion, status int, headers http.Header, body []byte) []Result {
	var out []Result
	for _, a := range assertions {
		if !a.Enabled {
			continue
		}
		out = append(out, evalOne(a, status, headers, body))
	}
	return out
}

func evalOne(a model.Assertion, status int, headers http.Header, body []byte) Result {
	name := strings.TrimSpace(a.Source + " " + a.Op + " " + a.Expected)
	fail := func(msg string) Result { return Result{Name: name, Passed: false, Error: msg} }
	pass := Result{Name: name, Passed: true}

	actual, found := extract(a.Source, status, headers, body)

	switch a.Op {
	case OpExists:
		if found {
			return pass
		}
		return fail("expected " + a.Source + " to exist")
	case OpNotExists:
		if !found {
			return pass
		}
		return fail("expected " + a.Source + " not to exist, got " + actual)
	}

	if !found {
		return fail(a.Source + " not found in response")
	}

	switch a.Op {
	case OpEquals:
		return cond(actual == a.Expected, name, fmt.Sprintf("expected %q to equal %q", actual, a.Expected))
	case OpNotEquals:
		return cond(actual != a.Expected, name, fmt.Sprintf("expected %q not to equal %q", actual, a.Expected))
	case OpContains:
		return cond(strings.Contains(actual, a.Expected), name, fmt.Sprintf("expected %q to contain %q", actual, a.Expected))
	case OpNotContains:
		return cond(!strings.Contains(actual, a.Expected), name, fmt.Sprintf("expected %q not to contain %q", actual, a.Expected))
	case OpMatches:
		re, err := regexp.Compile(a.Expected)
		if err != nil {
			return fail("invalid regular expression: " + err.Error())
		}
		return cond(re.MatchString(actual), name, fmt.Sprintf("expected %q to match /%s/", actual, a.Expected))
	case OpGreater, OpLess:
		af, err1 := strconv.ParseFloat(strings.TrimSpace(actual), 64)
		ef, err2 := strconv.ParseFloat(strings.TrimSpace(a.Expected), 64)
		if err1 != nil || err2 != nil {
			return fail(fmt.Sprintf("non-numeric comparison: %q vs %q", actual, a.Expected))
		}
		if a.Op == OpGreater {
			return cond(af > ef, name, fmt.Sprintf("expected %s to be greater than %s", actual, a.Expected))
		}
		return cond(af < ef, name, fmt.Sprintf("expected %s to be less than %s", actual, a.Expected))
	default:
		return fail("unknown operator: " + a.Op)
	}
}

func cond(ok bool, name, failMsg string) Result {
	if ok {
		return Result{Name: name, Passed: true}
	}
	return Result{Name: name, Passed: false, Error: failMsg}
}

// extract resolves a source expression against the response, returning the
// value and whether it was found. Supported sources:
//
//	res.status              -> the numeric status code
//	res.body                -> the raw response body
//	res.header.<Name>       -> a response header value (first)
//	res.json.<dotted.path>  -> a value inside a JSON body (array indices allowed)
func extract(source string, status int, headers http.Header, body []byte) (string, bool) {
	switch {
	case source == "res.status":
		return strconv.Itoa(status), true
	case source == "res.body":
		return string(body), true
	case strings.HasPrefix(source, "res.header."):
		name := source[len("res.header."):]
		if vs := headers.Values(name); len(vs) > 0 {
			return vs[0], true
		}
		return "", false
	case strings.HasPrefix(source, "res.json."):
		return jsonPath(body, source[len("res.json."):])
	default:
		return "", false
	}
}

// jsonPath walks a dotted path into a JSON body (numeric segments index arrays)
// and returns the addressed value as a string.
func jsonPath(body []byte, path string) (string, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return "", false
	}
	for _, seg := range strings.Split(path, ".") {
		switch cur := v.(type) {
		case map[string]any:
			nv, ok := cur[seg]
			if !ok {
				return "", false
			}
			v = nv
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				return "", false
			}
			v = cur[i]
		default:
			return "", false
		}
	}
	return scalarString(v), true
}

// scalarString renders a decoded JSON value as a comparison string.
func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
