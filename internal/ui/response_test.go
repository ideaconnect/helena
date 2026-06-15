package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// newResponseUI builds a headless MainUI (no collection) for response-viewer tests.
func newResponseUI(t *testing.T) *MainUI {
	t.Helper()
	test.NewApp()
	sess, err := session.New("")
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return NewMainUI(sess)
}

// TestApplyResponseJSONAutoDetectsStructured verifies a JSON body is fed to the
// PrettyView, auto-detected as JSON, with the Body tab selected and the status set.
func TestApplyResponseJSONAutoDetectsStructured(t *testing.T) {
	m := newResponseUI(t)
	body := `{"a":1,"b":[2,3]}`
	m.applyResponse(&tabResponse{rawBody: body, status: "200 OK"})

	if got := string(m.pv.Source()); got != body {
		t.Errorf("pv source = %q, want %q", got, body)
	}
	if m.pv.Format() != prettyview.FormatJSON {
		t.Errorf("format = %v, want FormatJSON", m.pv.Format())
	}
	if m.Response.SelectedIndex() != 0 {
		t.Errorf("selected response tab = %d, want 0 (Body)", m.Response.SelectedIndex())
	}
	if m.Status.Text != "200 OK" {
		t.Errorf("status = %q, want 200 OK", m.Status.Text)
	}
}

// TestApplyResponsePlainTextIsRaw verifies a non-structured body renders raw.
func TestApplyResponsePlainTextIsRaw(t *testing.T) {
	m := newResponseUI(t)
	body := "just a plain text response\nsecond line"
	m.applyResponse(&tabResponse{rawBody: body, status: "200"})
	if m.pv.Format() != prettyview.FormatRaw {
		t.Errorf("plain-text format = %v, want FormatRaw", m.pv.Format())
	}
	if got := string(m.pv.Source()); got != body {
		t.Errorf("pv source = %q, want %q", got, body)
	}
}

// TestApplyResponseMalformedDoesNotPanic verifies a broken/empty body renders
// without panicking (PrettyView always falls back to raw).
func TestApplyResponseMalformedDoesNotPanic(t *testing.T) {
	m := newResponseUI(t)
	for _, body := range []string{`{not valid json`, `<unclosed`, "", "\x00\x01binary"} {
		m.applyResponse(&tabResponse{rawBody: body, status: "200"})
		if got := string(m.pv.Source()); got != body {
			t.Errorf("pv source = %q, want %q", got, body)
		}
	}
}

// TestApplyResponseErrorIsRaw verifies an error response shows the error text as
// raw — even when it resembles JSON — and never as structured data.
func TestApplyResponseErrorIsRaw(t *testing.T) {
	m := newResponseUI(t)
	m.applyResponse(&tabResponse{isError: true, errText: `{"err":"boom"}`, status: "Error: boom"})
	if m.pv.Format() != prettyview.FormatRaw {
		t.Errorf("error format = %v, want FormatRaw (error text must not auto-structure)", m.pv.Format())
	}
	if got := string(m.pv.Source()); got != `{"err":"boom"}` {
		t.Errorf("pv source = %q", got)
	}
	if m.Status.Text != "Error: boom" {
		t.Errorf("status = %q, want Error: boom", m.Status.Text)
	}
}

// TestClearResponsePanelEmptiesViewer verifies a nil response clears the viewer.
func TestClearResponsePanelEmptiesViewer(t *testing.T) {
	m := newResponseUI(t)
	m.applyResponse(&tabResponse{rawBody: `{"a":1}`, status: "200"})
	m.applyResponse(nil)
	if got := string(m.pv.Source()); got != "" {
		t.Errorf("pv source after clear = %q, want empty", got)
	}
}

// TestVariantFor verifies the Helena-theme -> Fyne-variant mapping that drives
// PrettyView's theme repaint on a light/dark switch.
func TestVariantFor(t *testing.T) {
	test.NewApp()
	if got := variantFor(model.ThemeLight); got != theme.VariantLight {
		t.Errorf("variantFor(Light) = %v, want VariantLight", got)
	}
	if got := variantFor(model.ThemeDark); got != theme.VariantDark {
		t.Errorf("variantFor(Dark) = %v, want VariantDark", got)
	}
	_ = variantFor(model.ThemeSystem) // defers to the app variant; must not panic
}
