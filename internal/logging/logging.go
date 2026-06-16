// Package logging is Helena's diagnostic logging facility: a process-wide
// slog.Logger plus redaction helpers so a log file can capture send round-trips
// and recovered panics for field bug reports without ever leaking credentials.
//
// It is a no-op (discards everything) until Configure runs, so importing it is
// always safe and tests stay quiet by default.
package logging

import (
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
)

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

// L returns the process logger. Before Configure it discards everything.
func L() *slog.Logger { return logger }

// Configure installs the process logger. It always writes to stderr; when
// logFile is non-empty it also appends to that file (mode 0600, since logs may
// reference hosts/paths), returning a close func for it. verbose raises the
// level from Info to Debug. A nil-safe close func is always returned.
func Configure(verbose bool, logFile string) (func() error, error) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	var w io.Writer = os.Stderr
	closeFn := func() error { return nil }
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return closeFn, err
		}
		w = io.MultiWriter(os.Stderr, f)
		closeFn = f.Close
	}
	logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
	return closeFn, nil
}

// SetOutput points the logger at an arbitrary writer at the given verbosity.
// Used by tests to capture output; production wiring goes through Configure.
func SetOutput(w io.Writer, verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// sensitiveHeaders are header names whose values must never be logged verbatim.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
}

// RedactHeaderValue masks the value of a credential-bearing header
// (Authorization, API keys, cookies); other headers pass through unchanged.
func RedactHeaderValue(name, value string) string {
	if value != "" && sensitiveHeaders[strings.ToLower(strings.TrimSpace(name))] {
		return "[redacted]"
	}
	return value
}

// RedactURL strips userinfo and the query string from a URL so a logged request
// line can't leak embedded credentials (user:pass@host or a {{token}}-expanded
// query). The scheme/host/path stay for diagnosis; an unparseable input is
// fully redacted.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[redacted url]"
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	if u.RawQuery != "" {
		u.RawQuery = "redacted"
	}
	return u.String()
}
