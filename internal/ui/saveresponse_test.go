package ui

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestWriteResponseByteExact pins #66: the active response is written to the
// destination byte-for-byte, including NUL and high bytes (binary-safe).
func TestWriteResponseByteExact(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)

	payload := []byte{0x00, 0x01, 0xff, 0xfe, 'h', 'i', 0x0a, 0x80, 0x00}
	m.tabs = append(m.tabs, &openTab{requestID: "x", resp: &tabResponse{rawBody: string(payload)}})
	m.activeTabIdx = len(m.tabs) - 1

	var buf bytes.Buffer
	n, err := m.writeResponseTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Errorf("wrote %d bytes; want %d", n, len(payload))
	}
	if !bytes.Equal(buf.Bytes(), payload) {
		t.Errorf("response not byte-exact:\n got  %v\n want %v", buf.Bytes(), payload)
	}
}

// TestWriteResponseNoneIsError pins that there is nothing to save without a
// successful response (no tab / no response / an error placeholder).
func TestWriteResponseNoneIsError(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)

	if _, err := m.writeResponseTo(&bytes.Buffer{}); err == nil {
		t.Error("expected an error when no response is present")
	}

	m.tabs = append(m.tabs, &openTab{resp: &tabResponse{isError: true, errText: "boom"}})
	m.activeTabIdx = 0
	if _, ok := m.currentResponseBytes(); ok {
		t.Error("an error-placeholder response should not be saveable")
	}
}
