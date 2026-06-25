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

// runCommand executes `helena run <collection-dir> [--env NAME]` (#90): it loads
// the collection headlessly, runs every request (chain + scripts + assertions),
// prints a report, and returns the process exit code — 1 when any request
// errored or any check failed, 2 on a usage/setup error.
func runCommand(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	env := fs.String("env", "", "active environment name")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: helena run <collection-dir> [--env NAME]")
		return 2
	}

	sess, err := session.New("") // ephemeral session: no config persistence
	if err != nil {
		fmt.Fprintf(os.Stderr, "helena run: %v\n", err)
		return 2
	}
	if err := sess.OpenCollection(fs.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "helena run: open %q: %v\n", fs.Arg(0), err)
		return 2
	}
	sess.SetActiveCollection(0)
	if *env != "" {
		sess.SetActiveEnv(*env)
	}

	rep := runner.Run(context.Background(), sess)
	printReport(os.Stdout, rep)
	if rep.Failed() {
		return 1
	}
	return 0
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
