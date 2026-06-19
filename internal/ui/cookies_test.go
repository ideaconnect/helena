package ui

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/cookiejar"
	"github.com/idct/helena/internal/session"
)

// --- walk helpers (walkObjects lives in workspaces_test.go) ---

func ttButtons(o fyne.CanvasObject) []*ttwidget.Button {
	var bs []*ttwidget.Button
	walkObjects(o, func(c fyne.CanvasObject) {
		if b, ok := c.(*ttwidget.Button); ok {
			bs = append(bs, b)
		}
	})
	return bs
}

func entriesIn(o fyne.CanvasObject) []*widget.Entry {
	var es []*widget.Entry
	walkObjects(o, func(c fyne.CanvasObject) {
		if e, ok := c.(*widget.Entry); ok {
			es = append(es, e)
		}
	})
	return es
}

func checksIn(o fyne.CanvasObject) []*widget.Check {
	var cs []*widget.Check
	walkObjects(o, func(c fyne.CanvasObject) {
		if x, ok := c.(*widget.Check); ok {
			cs = append(cs, x)
		}
	})
	return cs
}

func listIn(o fyne.CanvasObject) *widget.List {
	var l *widget.List
	walkObjects(o, func(c fyne.CanvasObject) {
		if x, ok := c.(*widget.List); ok && l == nil {
			l = x
		}
	})
	return l
}

func buttonByText(o fyne.CanvasObject, text string) *widget.Button {
	var found *widget.Button
	walkObjects(o, func(c fyne.CanvasObject) {
		if b, ok := c.(*widget.Button); ok && b.Text == text {
			found = b
		}
	})
	return found
}

// newCookieUI builds a MainUI on a window with a seeded session jar.
func newCookieUI(t *testing.T) (*MainUI, fyne.Window, *cookiejar.Jar) {
	t.Helper()
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(800, 560))
	m.SetWindow(w)
	return m, w, s.CookieJar()
}

// TestCookiesDialogListsAndGates pins #91's viewer: stored cookies appear in the
// list and Edit/Delete stay disabled until a row is selected (Add + Clear are
// always enabled).
func TestCookiesDialogListsAndGates(t *testing.T) {
	m, w, jar := newCookieUI(t)
	defer w.Close()
	u, _ := url.Parse("https://example.com/")
	jar.SetCookies(u, []*http.Cookie{
		{Name: "sid", Value: "abc", Path: "/"},
		{Name: "theme", Value: "dark", Path: "/"},
	})

	m.showCookies()
	dlg := w.Canvas().Overlays().Top()
	if dlg == nil {
		t.Fatal("cookies dialog did not open")
	}
	buttons := ttButtons(dlg)
	list := listIn(dlg)
	if len(buttons) != 4 {
		t.Fatalf("expected 4 actions (Add/Edit/Delete/Clear), got %d", len(buttons))
	}
	if list == nil || list.Length() != 2 {
		t.Fatalf("cookie list should show 2 rows, got %v", list)
	}

	enabled := func() int {
		n := 0
		for _, b := range buttons {
			if !b.Disabled() {
				n++
			}
		}
		return n
	}
	if got := enabled(); got != 2 { // Add + Clear only
		t.Errorf("with no selection, %d enabled; want 2 (Add+Clear)", got)
	}
	list.Select(0)
	if got := enabled(); got != 4 {
		t.Errorf("after selection, %d enabled; want 4", got)
	}
	list.Unselect(0)
	if got := enabled(); got != 2 {
		t.Errorf("after unselect, %d enabled; want 2", got)
	}
}

// TestCookiesAddThroughDialog drives the real Add path: tap Add → fill the form →
// Save, and the cookie must land in the session jar (exercises showCookies' addBtn
// closure → jar.Set, not just editCookie in isolation).
func TestCookiesAddThroughDialog(t *testing.T) {
	m, w, jar := newCookieUI(t)
	defer w.Close()

	m.showCookies()
	dlg := w.Canvas().Overlays().Top()
	buttons := ttButtons(dlg)
	test.Tap(buttons[0]) // Add

	form := w.Canvas().Overlays().Top()
	if form == dlg {
		t.Fatal("Add did not open the cookie form")
	}
	es := entriesIn(form)
	if len(es) < 4 {
		t.Fatalf("form should have 4 entries, got %d", len(es))
	}
	es[0].SetText("api.example.com") // Domain
	es[2].SetText("token")           // Name
	es[3].SetText("xyz")             // Value
	save := buttonByText(form, "Save")
	if save == nil {
		t.Fatal("form Save button not found")
	}
	test.Tap(save)

	all := jar.All()
	if len(all) != 1 || all[0].Name != "token" || all[0].Domain != "api.example.com" || all[0].Value != "xyz" {
		t.Fatalf("Add did not store the cookie in the jar: %+v", all)
	}
}

// TestCookiesEditThroughDialog drives the real Edit path and pins three behaviours
// the reviewer flagged as untested: the Remove-then-Set orphan prevention on an
// identity change (rename), the Secure/HttpOnly checkbox plumbing, and the Expires
// carry-through (the form has no expiry input, so an edited persistent cookie must
// keep its expiry).
func TestCookiesEditThroughDialog(t *testing.T) {
	m, w, jar := newCookieUI(t)
	defer w.Close()

	expiry := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	jar.Set(cookiejar.Cookie{Domain: "example.com", Path: "/", Name: "old", Value: "v", Expires: expiry})

	m.showCookies()
	dlg := w.Canvas().Overlays().Top()
	list := listIn(dlg)
	list.Select(0)
	buttons := ttButtons(dlg)
	test.Tap(buttons[1]) // Edit

	form := w.Canvas().Overlays().Top()
	es := entriesIn(form)
	es[2].SetText("new") // rename (changes the (domain,path,name) identity)
	cs := checksIn(form)
	if len(cs) < 2 {
		t.Fatalf("form should expose Secure + HttpOnly checks, got %d", len(cs))
	}
	cs[0].SetChecked(true) // Secure
	cs[1].SetChecked(true) // HttpOnly
	test.Tap(buttonByText(form, "Save"))

	all := jar.All()
	if len(all) != 1 {
		t.Fatalf("rename should not orphan the old slot; want 1 cookie, got %d: %+v", len(all), all)
	}
	got := all[0]
	if got.Name != "new" {
		t.Fatalf("rename failed: %+v", got)
	}
	if !got.Secure || !got.HTTPOnly {
		t.Fatalf("Secure/HttpOnly checkboxes not applied: %+v", got)
	}
	if !got.Expires.Equal(expiry) {
		t.Fatalf("Expires not carried through edit: got %v want %v", got.Expires, expiry)
	}
}

// TestCookiesDeleteAndClearThroughDialog drives the real Delete and Clear paths.
func TestCookiesDeleteAndClearThroughDialog(t *testing.T) {
	m, w, jar := newCookieUI(t)
	defer w.Close()
	jar.Set(cookiejar.Cookie{Domain: "a.com", Path: "/", Name: "x", Value: "1"})
	jar.Set(cookiejar.Cookie{Domain: "b.com", Path: "/", Name: "y", Value: "1"})

	m.showCookies()
	dlg := w.Canvas().Overlays().Top()
	list := listIn(dlg)
	buttons := ttButtons(dlg)

	list.Select(0)
	test.Tap(buttons[2]) // Delete
	if jar.Len() != 1 {
		t.Fatalf("Delete should leave 1 cookie, got %d", jar.Len())
	}

	test.Tap(buttons[3]) // Clear → confirm dialog
	confirm := w.Canvas().Overlays().Top()
	yes := buttonByText(confirm, "Yes")
	if yes == nil {
		t.Fatal("Clear confirm dialog has no Yes button")
	}
	test.Tap(yes)
	if jar.Len() != 0 {
		t.Fatalf("Clear should empty the jar, got %d", jar.Len())
	}
}

func TestCookieSummary(t *testing.T) {
	expiry := time.Date(2026, 3, 4, 5, 6, 0, 0, time.UTC)
	cases := []struct {
		c    cookiejar.Cookie
		want []string // substrings that must appear
	}{
		{cookiejar.Cookie{Domain: "example.com", Name: "a", Value: "1", Path: "/"}, []string{"example.com", "a=1", "session"}},
		{cookiejar.Cookie{Domain: "x.com", Name: "s", Value: "v", Path: "/api", Secure: true, HTTPOnly: true}, []string{"path /api", "secure", "httpOnly"}},
		{cookiejar.Cookie{Domain: "x.com", Name: "e", Value: "v", Path: "/", Expires: expiry}, []string{"expires 2026-03-04"}},
	}
	for _, tc := range cases {
		got := cookieSummary(tc.c)
		for _, sub := range tc.want {
			if !strings.Contains(got, sub) {
				t.Errorf("summary %q missing %q", got, sub)
			}
		}
	}
}
