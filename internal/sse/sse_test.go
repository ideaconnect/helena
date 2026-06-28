package sse

import (
	"io"
	"strings"
	"testing"
)

// collect drains a parser into a slice.
func collect(t *testing.T, stream string) []Event {
	t.Helper()
	p := NewParser(strings.NewReader(stream))
	var out []Event
	for {
		ev, err := p.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		out = append(out, ev)
	}
}

// TestParseBasic covers the canonical fields: event type, multi-line data
// (joined by \n, trailing newline trimmed), id persistence, and the default
// "message" type.
func TestParseBasic(t *testing.T) {
	stream := "" +
		"event: greeting\n" +
		"id: 1\n" +
		"data: hello\n" +
		"data: world\n" +
		"\n" +
		"data: just-data\n" +
		"\n"
	got := collect(t, stream)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Type != "greeting" || got[0].Data != "hello\nworld" || got[0].ID != "1" {
		t.Errorf("event 0 = %+v", got[0])
	}
	// id persists; type defaults to "message".
	if got[1].Type != "message" || got[1].Data != "just-data" || got[1].ID != "1" {
		t.Errorf("event 1 = %+v", got[1])
	}
}

// TestParseCommentsAndCRLF verifies comment lines are ignored and \r\n / lone \r
// terminators work.
func TestParseCommentsAndCRLF(t *testing.T) {
	stream := ": this is a comment\r\n" +
		"data: a\r\n" +
		"\r\n" +
		"data: b\r" +
		"\r"
	got := collect(t, stream)
	if len(got) != 2 || got[0].Data != "a" || got[1].Data != "b" {
		t.Fatalf("CRLF/comment parse = %+v", got)
	}
}

// TestParseRetryAndNoSpace covers a numeric retry (persisted onto events) and a
// field with no space after the colon plus a value-less field.
func TestParseRetryAndNoSpace(t *testing.T) {
	stream := "retry: 4000\n" +
		"data:nospace\n" +
		"\n"
	got := collect(t, stream)
	if len(got) != 1 || got[0].Data != "nospace" || got[0].Retry != 4000 {
		t.Fatalf("retry/no-space parse = %+v", got)
	}
}

// TestParseBlankDataNoDispatch verifies a blank line with no buffered data does
// not dispatch an event (only resets), and a bare field name yields empty value.
func TestParseBlankDataNoDispatch(t *testing.T) {
	stream := "event: ping\n" + // event type set but no data
		"\n" + // should NOT dispatch
		"data\n" + // bare field name → data value ""
		"data: x\n" +
		"\n"
	got := collect(t, stream)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	// The first (empty-data) event was dropped; the surviving event's type is the
	// default since the `event: ping` was reset by the non-dispatching blank line.
	if got[0].Type != "message" || got[0].Data != "\nx" {
		t.Errorf("event = %+v", got[0])
	}
}

// TestParseNoTrailingBlankDropsLast verifies an event not terminated by a blank
// line before EOF is discarded (per the spec, EOF does not dispatch).
func TestParseNoTrailingBlankDropsLast(t *testing.T) {
	got := collect(t, "data: done\n\ndata: pending-no-blank")
	if len(got) != 1 || got[0].Data != "done" {
		t.Errorf("EOF should not dispatch the pending event: %+v", got)
	}
}
