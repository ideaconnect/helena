// Package sse parses Server-Sent Events (text/event-stream) streams (#74),
// following the WHATWG "Server-Sent Events" interpretation algorithm. It is a
// pure stream→events transform: no network, no UI. internal/httpclient drives a
// streaming request and feeds the response body here; the UI renders the events.
package sse

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Event is one dispatched Server-Sent Event.
type Event struct {
	// ID is the stream's last event id (persists across events until a new
	// `id:` field arrives), per the spec.
	ID string
	// Type is the event type — the `event:` field, defaulting to "message".
	Type string
	// Data is the accumulated `data:` payload with the trailing newline removed.
	Data string
	// Retry is the reconnection time in milliseconds the server last advertised
	// via a `retry:` field (0 when never set).
	Retry int
}

// Parser yields events from an event-stream reader via repeated Next calls.
type Parser struct {
	sc     *bufio.Scanner
	lastID string // persists across events (WHATWG "last event ID buffer")
	retry  int    // persists across events
}

// NewParser returns a Parser reading the event stream from r.
func NewParser(r io.Reader) *Parser {
	sc := bufio.NewScanner(r)
	sc.Split(scanLines)
	// Allow long single lines (e.g. a big JSON data: payload) up to 1 MiB.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return &Parser{sc: sc}
}

// Next returns the next dispatched event, or io.EOF when the stream ends. A
// blank line with no buffered data resets the in-progress fields without
// dispatching (per the spec), so Next only returns events that carry data.
func (p *Parser) Next() (Event, error) {
	var data strings.Builder
	var eventType string
	hasData := false

	for p.sc.Scan() {
		line := p.sc.Text()
		if line == "" { // blank line → dispatch
			if !hasData {
				eventType = ""
				data.Reset()
				continue
			}
			payload := strings.TrimSuffix(data.String(), "\n")
			et := eventType
			if et == "" {
				et = "message"
			}
			return Event{ID: p.lastID, Type: et, Data: payload, Retry: p.retry}, nil
		}
		if strings.HasPrefix(line, ":") { // comment
			continue
		}
		field, value := splitField(line)
		switch field {
		case "event":
			eventType = value
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
			hasData = true
		case "id":
			if !strings.ContainsRune(value, 0) { // a NUL in the id is ignored
				p.lastID = value
			}
		case "retry":
			if n, err := strconv.Atoi(value); err == nil {
				p.retry = n
			}
		}
	}
	if err := p.sc.Err(); err != nil {
		return Event{}, err
	}
	return Event{}, io.EOF
}

// splitField splits an event-stream line into field name and value. A line with
// no colon is a field name with an empty value; a single leading space after the
// colon is stripped.
func splitField(line string) (field, value string) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.TrimPrefix(line[i+1:], " ")
}

// scanLines is a bufio.SplitFunc that treats \n, \r, and \r\n all as line
// terminators (the event-stream spec accepts any of the three).
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 < len(data) {
				if data[i+1] == '\n' {
					return i + 2, data[:i], nil
				}
				return i + 1, data[:i], nil
			}
			if atEOF {
				return i + 1, data[:i], nil
			}
			return 0, nil, nil // need one more byte to check for \n
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
