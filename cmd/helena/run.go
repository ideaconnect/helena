package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/idct/helena/internal/runner"
	"github.com/idct/helena/internal/session"
)

// runCommand executes `helena run <collection-dir> [--env NAME] [--format FMT]`
// (#90): it loads the collection headlessly, runs every request (chain + scripts
// + assertions), writes a report in the chosen format (text / json / junit), and
// returns the process exit code — 1 when any request errored or any check
// failed, 2 on a usage/setup error.
func runCommand(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	env := fs.String("env", "", "active environment name")
	format := fs.String("format", "text", "output format: text, json, or junit")
	fs.SetOutput(os.Stderr)
	// Two-phase parse so flags may appear before OR after the <collection-dir>
	// positional. Go's flag package stops at the first non-flag argument, so a
	// single fs.Parse would leave flags written after the dir (the natural
	// `helena run ./col --env Staging` form the README documents) unparsed.
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: helena run <collection-dir> [--env NAME] [--format text|json|junit]")
		return 2
	}
	dir := rest[0]
	if err := fs.Parse(rest[1:]); err != nil { // flags that followed the dir
		return 2
	}
	// Validate the format up front so a typo fails before the (network) run.
	switch *format {
	case "text", "json", "junit":
	default:
		fmt.Fprintf(os.Stderr, "helena run: unknown --format %q (want text, json, or junit)\n", *format)
		return 2
	}

	sess, err := session.New("") // ephemeral session: no config persistence
	if err != nil {
		fmt.Fprintf(os.Stderr, "helena run: %v\n", err)
		return 2
	}
	if err := sess.OpenCollection(dir); err != nil {
		fmt.Fprintf(os.Stderr, "helena run: open %q: %v\n", dir, err)
		return 2
	}
	sess.SetActiveCollection(0)
	if *env != "" {
		sess.SetActiveEnv(*env)
	}

	rep := runner.Run(context.Background(), sess)
	if err := writeReport(os.Stdout, rep, *format); err != nil {
		fmt.Fprintf(os.Stderr, "helena run: %v\n", err)
		return 2
	}
	if rep.Failed() {
		return 1
	}
	return 0
}

// writeReport renders rep to w in the chosen format. text is the human-readable
// summary; json and junit are machine-readable (runner.Report.JSON / .JUnit).
func writeReport(w io.Writer, rep runner.Report, format string) error {
	switch format {
	case "json":
		b, err := rep.JSON()
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	case "junit":
		b, err := rep.JUnit()
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	default:
		printReport(w, rep)
		return nil
	}
}

// printReport renders a run report as plain text: one line per request with its
// status, each check below it, and a totals summary.
func printReport(w io.Writer, rep runner.Report) {
	for _, r := range rep.Results {
		status := "—"
		if r.StatusCode != 0 {
			status = fmt.Sprintf("%d", r.StatusCode)
		}
		mark := "ok  "
		if r.Skipped {
			mark = "SKIP"
		}
		if !r.OK() {
			mark = "FAIL"
		}
		fmt.Fprintf(w, "%s  %-28s %-6s %-4s %s\n", mark, r.Path, r.Method, status, r.Duration.Round(time.Millisecond))
		if r.Err != "" {
			fmt.Fprintf(w, "      error: %s\n", r.Err)
		}
		for _, c := range r.Checks {
			cm := "PASS"
			if !c.Passed {
				cm = "FAIL"
			}
			line := "      " + cm + "  " + c.Name
			if !c.Passed && c.Error != "" {
				line += " — " + c.Error
			}
			fmt.Fprintln(w, line)
		}
	}
	reqs, passed, failed := rep.Totals()
	fmt.Fprintf(w, "\n%d requests, %d checks passed, %d failed\n", reqs, passed, failed)
}
