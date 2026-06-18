package vars

import (
	"crypto/rand"
	"fmt"
	mrand "math/rand/v2"
	"strconv"
	"strings"
	"time"
)

// Dynamic resolves Postman-style dynamic ("magic") variables — names beginning
// with "$" such as {{$guid}}, {{$timestamp}}, {{$randomInt}}. Attach it as a
// Resolver fallback (alone via WithFallback, or with Compose alongside others).
// An unknown $name returns ("", false) so the resolver still reports it missing;
// non-$ names are ignored for the same reason. Each call generates a fresh value
// (a value is frozen by the resolver and never re-expanded).
func Dynamic(name string) (string, bool) {
	if !strings.HasPrefix(name, "$") {
		return "", false
	}
	switch name {
	case "$guid", "$randomUUID":
		return uuidV4(), true
	case "$timestamp":
		return strconv.FormatInt(time.Now().Unix(), 10), true
	case "$isoTimestamp":
		return time.Now().UTC().Format(time.RFC3339), true
	case "$randomInt":
		return strconv.Itoa(mrand.IntN(1001)), true // Postman: 0..1000 inclusive
	case "$randomFloat":
		return strconv.FormatFloat(mrand.Float64(), 'f', -1, 64), true
	case "$randomBoolean":
		return strconv.FormatBool(mrand.IntN(2) == 1), true
	case "$randomFirstName":
		return pick(firstNames), true
	case "$randomLastName":
		return pick(lastNames), true
	case "$randomFullName":
		return pick(firstNames) + " " + pick(lastNames), true
	case "$randomEmail":
		return strings.ToLower(pick(firstNames) + "." + pick(lastNames) + "@example.com"), true
	case "$randomColor":
		return pick(colors), true
	}
	return "", false
}

// Compose returns a fallback that consults each lookup in order and returns the
// first match; nil lookups are skipped. Use it to attach several fallbacks to a
// single Resolver — e.g. Compose(chain.VarLookup(...), vars.Dynamic).
func Compose(lookups ...func(string) (string, bool)) func(string) (string, bool) {
	return func(name string) (string, bool) {
		for _, fn := range lookups {
			if fn == nil {
				continue
			}
			if v, ok := fn(name); ok {
				return v, true
			}
		}
		return "", false
	}
}

func pick(s []string) string { return s[mrand.IntN(len(s))] }

// uuidV4 returns a random RFC 4122 version-4 UUID string.
func uuidV4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Curated faker tables for the $random*Name / $randomColor generators — kept
// small and dependency-free (no faker library).
var (
	firstNames = []string{"Ada", "Alan", "Grace", "Linus", "Ken", "Dennis", "Margaret", "Edsger", "Barbara", "Donald"}
	lastNames  = []string{"Lovelace", "Turing", "Hopper", "Torvalds", "Thompson", "Ritchie", "Hamilton", "Dijkstra", "Liskov", "Knuth"}
	colors     = []string{"red", "green", "blue", "cyan", "magenta", "yellow", "black", "white", "orange", "purple"}
)
