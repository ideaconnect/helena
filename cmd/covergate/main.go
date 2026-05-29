// covergate is a small Go test-coverage analyser used by Helena's
// Phase 8 testing pass and the CI coverage gate.
//
// Given a coverage profile produced by `go test -coverprofile=...`, it
// computes per-package statement coverage (count-weighted), prints a
// table, and optionally enforces a per-package floor — exiting
// non-zero if any non-excluded package is below the floor.
//
// Usage:
//
//	covergate -profile coverage.out
//	covergate -profile coverage.out -floor 90 -exclude internal/ui,cmd
//
// The exclude flag matches against the package's import-path suffix
// (after the module path), so `internal/ui` matches both the package
// itself and any future sub-packages.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// pkgStats accumulates statement counts for one Go package.
type pkgStats struct {
	total   int
	covered int
}

func main() {
	profile := flag.String("profile", "coverage.out", "path to the coverage profile (from go test -coverprofile)")
	floor := flag.Float64("floor", 0, "minimum per-package coverage percentage (0 = no gate; just print)")
	excludeRaw := flag.String("exclude", "", "comma-separated package-path suffixes to exclude from the floor (still printed, marked SKIP)")
	flag.Parse()

	excludes := splitCSV(*excludeRaw)

	stats, err := readProfile(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covergate: %v\n", err)
		os.Exit(2)
	}
	if len(stats) == 0 {
		fmt.Fprintln(os.Stderr, "covergate: no coverage data in profile")
		os.Exit(2)
	}

	pkgs := make([]string, 0, len(stats))
	for p := range stats {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	var (
		totalAll, coveredAll int
		violations           int
		nameWidth            = 0
	)
	for _, p := range pkgs {
		if len(p) > nameWidth {
			nameWidth = len(p)
		}
	}
	if nameWidth < 7 {
		nameWidth = 7
	}

	fmt.Printf("%-*s  %8s  %8s  %s\n", nameWidth, "PACKAGE", "STMTS", "COV%", "STATUS")
	for _, p := range pkgs {
		s := stats[p]
		pct := 0.0
		if s.total > 0 {
			pct = float64(s.covered) / float64(s.total) * 100
		}
		totalAll += s.total
		coveredAll += s.covered

		status := "OK"
		switch {
		case isExcluded(p, excludes):
			status = "SKIP"
		case *floor > 0 && pct < *floor:
			status = fmt.Sprintf("FAIL  (< %.1f%%)", *floor)
			violations++
		}
		fmt.Printf("%-*s  %8d  %7.1f%%  %s\n", nameWidth, p, s.total, pct, status)
	}

	overall := 0.0
	if totalAll > 0 {
		overall = float64(coveredAll) / float64(totalAll) * 100
	}
	fmt.Println(strings.Repeat("-", nameWidth+34))
	fmt.Printf("%-*s  %8d  %7.1f%%\n", nameWidth, "TOTAL (all packages)", totalAll, overall)

	if *floor > 0 {
		if violations > 0 {
			fmt.Fprintf(os.Stderr, "\ncovergate: %d package(s) below %.1f%% floor\n", violations, *floor)
			os.Exit(1)
		}
		fmt.Printf("\ncovergate: every gated package meets the %.1f%% floor\n", *floor)
	}
}

// readProfile parses a Go cover profile and aggregates per-package
// statement counts. The profile's line format is:
//
//	<importpath>/<file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
//
// We sum NumStmts and conditionally NumStmts*1{count>0} per importpath
// directory.
func readProfile(filename string) (map[string]*pkgStats, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	defer f.Close()

	stats := map[string]*pkgStats{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// Split off the file:range; the remainder is "<stmts> <count>".
		spaceIdx := strings.IndexByte(line, ' ')
		if spaceIdx < 0 {
			return nil, fmt.Errorf("malformed line: %q", line)
		}
		filePart := line[:spaceIdx]
		rest := strings.Fields(line[spaceIdx+1:])
		if len(rest) != 2 {
			return nil, fmt.Errorf("malformed counts in line: %q", line)
		}
		colonIdx := strings.LastIndexByte(filePart, ':')
		if colonIdx < 0 {
			return nil, fmt.Errorf("missing file:range separator: %q", line)
		}
		pkg := path.Dir(filePart[:colonIdx])
		numStmts, err := strconv.Atoi(rest[0])
		if err != nil {
			return nil, fmt.Errorf("stmts not int in %q: %w", line, err)
		}
		count, err := strconv.Atoi(rest[1])
		if err != nil {
			return nil, fmt.Errorf("count not int in %q: %w", line, err)
		}
		s := stats[pkg]
		if s == nil {
			s = &pkgStats{}
			stats[pkg] = s
		}
		s.total += numStmts
		if count > 0 {
			s.covered += numStmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return stats, nil
}

// isExcluded reports whether pkg matches any suffix in excludes. The
// match is suffix-based so "internal/ui" excludes both the package
// itself and "internal/ui/subwidget".
func isExcluded(pkg string, excludes []string) bool {
	for _, ex := range excludes {
		if strings.HasSuffix(pkg, ex) || strings.Contains(pkg, "/"+ex+"/") {
			return true
		}
	}
	return false
}

// splitCSV splits a comma-separated string into trimmed non-empty
// entries.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
