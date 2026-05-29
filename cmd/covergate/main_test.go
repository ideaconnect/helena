package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadProfileAggregatesPerPackage verifies that readProfile sums
// statement counts per import-path directory and credits coverage only
// when a block's hit count is > 0.
func TestReadProfileAggregatesPerPackage(t *testing.T) {
	const profile = `mode: atomic
example.com/foo/a.go:1.1,3.2 5 1
example.com/foo/a.go:4.1,6.2 3 0
example.com/foo/b.go:1.1,2.2 2 1
example.com/bar/c.go:1.1,2.2 4 0
example.com/bar/c.go:3.1,4.2 1 1
`
	dir := t.TempDir()
	path := filepath.Join(dir, "cov.out")
	if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	stats, err := readProfile(path)
	if err != nil {
		t.Fatalf("readProfile: %v", err)
	}
	foo := stats["example.com/foo"]
	bar := stats["example.com/bar"]
	if foo == nil || bar == nil {
		t.Fatalf("missing packages: %+v", stats)
	}
	// foo: total=10 (5+3+2), covered=7 (5+0+2)
	if foo.total != 10 || foo.covered != 7 {
		t.Errorf("foo = {total:%d covered:%d}, want {10 7}", foo.total, foo.covered)
	}
	// bar: total=5 (4+1), covered=1
	if bar.total != 5 || bar.covered != 1 {
		t.Errorf("bar = {total:%d covered:%d}, want {5 1}", bar.total, bar.covered)
	}
}

// TestReadProfileRejectsMalformed verifies that lines missing the
// expected fields surface a clear parse error rather than silently
// crediting wrong counts.
func TestReadProfileRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"missing space": "mode: atomic\nnoseparator\n",
		"bad counts":    "mode: atomic\nexample.com/x.go:1.1,2.2 notanumber 0\n",
		"missing colon": "mode: atomic\nexample.com/x.go 5 1\n",
		"count not int": "mode: atomic\nexample.com/x.go:1.1,2.2 5 nope\n",
	}
	for name, body := range cases {
		path := filepath.Join(t.TempDir(), "p.out")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}
		if _, err := readProfile(path); err == nil {
			t.Errorf("%s: expected parse error, got nil", name)
		}
	}
}

// TestIsExcludedMatchesSuffixAndContains verifies the exclude rule's
// two intended matches: a package whose import path ends with the
// pattern, and a package whose path contains the pattern as an
// interior segment.
func TestIsExcludedMatchesSuffixAndContains(t *testing.T) {
	excludes := []string{"internal/ui", "cmd"}
	cases := map[string]bool{
		"github.com/idct/helena/internal/ui":        true,  // exact suffix
		"github.com/idct/helena/internal/ui/sub":    true,  // interior segment
		"github.com/idct/helena/cmd/helena":         true,  // interior
		"github.com/idct/helena/cmd":                true,  // exact suffix
		"github.com/idct/helena/internal/chain":     false, // unrelated
		"github.com/idct/helena/internal/uithelper": false, // partial-word substring NOT a path segment
	}
	for pkg, want := range cases {
		if got := isExcluded(pkg, excludes); got != want {
			t.Errorf("isExcluded(%q) = %v, want %v", pkg, got, want)
		}
	}
}

// TestSplitCSVTrimsAndDropsEmpty verifies the CLI's comma-separated
// list parser tolerates whitespace and trailing/empty entries.
func TestSplitCSVTrimsAndDropsEmpty(t *testing.T) {
	cases := map[string][]string{
		"":        nil,
		"a":       {"a"},
		"a,b":     {"a", "b"},
		" a , b ": {"a", "b"},
		"a,,b,":   {"a", "b"},
		"   ":     nil,
	}
	for in, want := range cases {
		got := splitCSV(in)
		if !equalStrSlice(got, want) {
			t.Errorf("splitCSV(%q) = %v, want %v", in, got, want)
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
