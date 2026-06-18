package vars

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestDynamicGUIDFormat(t *testing.T) {
	for _, name := range []string{"$guid", "$randomUUID"} {
		v, ok := Dynamic(name)
		if !ok {
			t.Fatalf("%s did not resolve", name)
		}
		if !uuidV4Re.MatchString(v) {
			t.Errorf("%s = %q; not a v4 UUID", name, v)
		}
	}
	// Two calls produce different values.
	a, _ := Dynamic("$guid")
	b, _ := Dynamic("$guid")
	if a == b {
		t.Error("$guid should be fresh each call")
	}
}

func TestDynamicTimestamps(t *testing.T) {
	v, ok := Dynamic("$timestamp")
	if !ok {
		t.Fatal("$timestamp did not resolve")
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		t.Fatalf("$timestamp = %q, not an int: %v", v, err)
	}
	if d := time.Since(time.Unix(n, 0)); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("$timestamp %d is not within ~now (delta %v)", n, d)
	}

	iso, ok := Dynamic("$isoTimestamp")
	if !ok {
		t.Fatal("$isoTimestamp did not resolve")
	}
	if _, err := time.Parse(time.RFC3339, iso); err != nil {
		t.Errorf("$isoTimestamp = %q, not RFC3339: %v", iso, err)
	}
}

func TestDynamicRandomInt(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		v, ok := Dynamic("$randomInt")
		if !ok {
			t.Fatal("$randomInt did not resolve")
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			t.Fatalf("$randomInt = %q out of [0,1000]", v)
		}
		seen[v] = true
	}
	if len(seen) < 2 {
		t.Error("$randomInt should vary across calls")
	}
}

func TestDynamicMisc(t *testing.T) {
	if v, ok := Dynamic("$randomFloat"); !ok {
		t.Error("$randomFloat did not resolve")
	} else if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 || f >= 1 {
		t.Errorf("$randomFloat = %q out of [0,1)", v)
	}
	if v, ok := Dynamic("$randomBoolean"); !ok || (v != "true" && v != "false") {
		t.Errorf("$randomBoolean = %q (ok=%v)", v, ok)
	}
	if v, ok := Dynamic("$randomFullName"); !ok || len(v) < 3 {
		t.Errorf("$randomFullName = %q (ok=%v)", v, ok)
	}
	if v, ok := Dynamic("$randomEmail"); !ok || !regexp.MustCompile(`^[a-z]+\.[a-z]+@example\.com$`).MatchString(v) {
		t.Errorf("$randomEmail = %q (ok=%v)", v, ok)
	}
	for _, n := range []string{"$randomFirstName", "$randomLastName", "$randomColor"} {
		if v, ok := Dynamic(n); !ok || v == "" {
			t.Errorf("%s = %q (ok=%v)", n, v, ok)
		}
	}
}

func TestDynamicUnknownAndNonDollar(t *testing.T) {
	if _, ok := Dynamic("$nope"); ok {
		t.Error("unknown $name should not resolve (must report as missing)")
	}
	if _, ok := Dynamic("plain"); ok {
		t.Error("non-$ name should be ignored by Dynamic")
	}
}

func TestCompose(t *testing.T) {
	a := func(n string) (string, bool) {
		if n == "x" {
			return "A", true
		}
		return "", false
	}
	b := func(n string) (string, bool) {
		if n == "x" || n == "y" {
			return "B", true
		}
		return "", false
	}
	c := Compose(nil, a, b)
	if v, ok := c("x"); !ok || v != "A" { // first match wins
		t.Errorf("compose x = %q (ok=%v); want A", v, ok)
	}
	if v, ok := c("y"); !ok || v != "B" {
		t.Errorf("compose y = %q (ok=%v); want B", v, ok)
	}
	if _, ok := c("z"); ok {
		t.Error("compose z should not resolve")
	}
}

func TestResolverWithDynamicFallback(t *testing.T) {
	r := New(map[string]string{"host": "api.test"}).WithFallback(Dynamic)
	out, missing := r.Resolve("https://{{host}}/x?t={{$guid}}&bad={{$nope}}")
	if !uuidV4Re.MatchString(extractAfter(out, "t=")) {
		t.Errorf("dynamic var not substituted: %q", out)
	}
	if len(missing) != 1 || missing[0] != "$nope" {
		t.Errorf("missing = %v; want [$nope]", missing)
	}
}

// extractAfter returns the substring after marker up to the next '&'.
func extractAfter(s, marker string) string {
	i := indexOf(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	if j := indexOf(rest, "&"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
