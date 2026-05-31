package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/responsefmt"
)

// lineText concatenates a display line's run texts back into a string.
func lineText(line []styledRun) string {
	var b strings.Builder
	for _, r := range line {
		b.WriteString(r.text)
	}
	return b.String()
}

// joinLines rebuilds the full document from wrapped/logical lines by joining
// run text with newlines between lines.
func joinLines(lines [][]styledRun) string {
	parts := make([]string, len(lines))
	for i, ln := range lines {
		parts[i] = lineText(ln)
	}
	return strings.Join(parts, "\n")
}

// TestWrapLinesPassthrough verifies short lines and cols<=0 are returned as-is.
func TestWrapLinesPassthrough(t *testing.T) {
	in := [][]styledRun{{{text: "abc"}}, {{text: "de"}}}
	if got := wrapLines(in, 0); len(got) != 2 {
		t.Fatalf("cols<=0 should pass through, got %d lines", len(got))
	}
	got := wrapLines(in, 10)
	if len(got) != 2 || lineText(got[0]) != "abc" || lineText(got[1]) != "de" {
		t.Fatalf("short lines should be unchanged, got %q", joinLines(got))
	}
}

// TestWrapLinesSplitsLongLine verifies a single line longer than cols is split
// into ceil(len/cols) display lines whose concatenation reproduces the line.
func TestWrapLinesSplitsLongLine(t *testing.T) {
	in := [][]styledRun{{{text: "abcdefg"}}}
	got := wrapLines(in, 3)
	want := []string{"abc", "def", "g"}
	if len(got) != len(want) {
		t.Fatalf("got %d display lines, want %d: %q", len(got), len(want), joinLines(got))
	}
	for i, w := range want {
		if lineText(got[i]) != w {
			t.Errorf("line %d = %q, want %q", i, lineText(got[i]), w)
		}
	}
}

// TestWrapLinesPreservesColorsAcrossCut verifies a wrap that falls inside a
// colored run keeps each fragment's color.
func TestWrapLinesPreservesColorsAcrossCut(t *testing.T) {
	red := responsefmt.TokenString
	num := responsefmt.TokenNumber
	// Two runs: "keyy" (string color) + "val" (number color) = "keyyval";
	// wrapping at 3 yields "key" / "yva" / "l".
	in := [][]styledRun{{
		{text: "keyy", col: tokenColor(red)},
		{text: "val", col: tokenColor(num)},
	}}
	got := wrapLines(in, 3)
	if joinLines(got) != "key\nyva\nl" {
		t.Fatalf("unexpected wrap layout: %q", joinLines(got))
	}
	// First fragment keeps the string color across the cut.
	if got[0][0].col != tokenColor(red) {
		t.Error("first fragment lost its color across the cut")
	}
	// The trailing "l" fragment keeps the number color across the cut.
	if last := got[2][len(got[2])-1]; last.col != tokenColor(num) {
		t.Error("number run lost its color across the cut")
	}
}

// TestWrapLinesKeepsEmptyLines verifies blank source lines survive as blank
// display lines (so the body's vertical structure is preserved).
func TestWrapLinesKeepsEmptyLines(t *testing.T) {
	in := [][]styledRun{{{text: "a"}}, nil, {{text: "b"}}}
	got := wrapLines(in, 5)
	if len(got) != 3 || lineText(got[1]) != "" {
		t.Fatalf("empty line not preserved: %q (%d lines)", joinLines(got), len(got))
	}
}

// TestBodyViewSetTextSplitsLines verifies SetText turns a multi-line string into
// one logical line per '\n' and records the full text for copy.
func TestBodyViewSetTextSplitsLines(t *testing.T) {
	test.NewApp()
	b := newBodyView("placeholder")
	b.SetText("one\ntwo\n")
	if len(b.logical) != 3 { // "one", "two", "" (trailing newline)
		t.Fatalf("logical lines = %d, want 3", len(b.logical))
	}
	if b.full != "one\ntwo\n" {
		t.Errorf("full = %q, want the original text", b.full)
	}
}

// TestBodyViewSetTextEmptyShowsPlaceholder verifies clearing falls back to the
// greyed placeholder line and forgets the copy text.
func TestBodyViewSetTextEmptyShowsPlaceholder(t *testing.T) {
	test.NewApp()
	b := newBodyView("nothing yet")
	b.SetText("data")
	b.SetText("")
	if b.full != "" {
		t.Errorf("full = %q, want empty after clear", b.full)
	}
	if len(b.logical) != 1 || lineText(b.logical[0]) != "nothing yet" {
		t.Errorf("placeholder not shown after clear: %q", joinLines(b.logical))
	}
}

// TestBodyViewCopyAll verifies copyAll writes the full body to the clipboard.
func TestBodyViewCopyAll(t *testing.T) {
	a := test.NewApp()
	b := newBodyView("")
	b.SetText("copy me\nplease")
	b.copyAll()
	if got := a.Clipboard().Content(); got != "copy me\nplease" {
		t.Errorf("clipboard = %q, want the full body", got)
	}
}

// TestBodyViewTypedShortcutCopies verifies Ctrl+C routes through to a copy.
func TestBodyViewTypedShortcutCopies(t *testing.T) {
	a := test.NewApp()
	b := newBodyView("")
	b.SetText("shortcut body")
	b.TypedShortcut(&fyne.ShortcutCopy{})
	if got := a.Clipboard().Content(); got != "shortcut body" {
		t.Errorf("clipboard = %q, want body after Ctrl+C", got)
	}
}

// TestCopyActiveResponseSelectsTabText verifies the Copy button copies the text
// of whichever Response tab is active.
func TestCopyActiveResponseSelectsTabText(t *testing.T) {
	m := newResponseTestUI(t)
	// The send flow sets the Raw bytes and headers separately from
	// renderResponseBody.
	m.responseRaw.SetText(`{"a":1}`)
	m.headersText.SetText("X: y")
	m.renderResponseBody([]byte(`{"a":1}`), "application/json")
	app := fyne.CurrentApp()

	m.Response.SelectIndex(1) // Raw → raw bytes
	m.copyActiveResponse()
	if got := app.Clipboard().Content(); got != `{"a":1}` {
		t.Errorf("Raw copy = %q, want raw bytes", got)
	}

	m.Response.SelectIndex(2) // Headers → header dump
	m.copyActiveResponse()
	if got := app.Clipboard().Content(); got != "X: y" {
		t.Errorf("Headers copy = %q, want header dump", got)
	}
}
