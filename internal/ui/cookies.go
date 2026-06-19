package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/cookiejar"
)

// showCookies opens the cookie-jar viewer/editor (#91): a list of the session
// jar's stored cookies with add / edit / delete and a clear-all, mirroring the
// workspaces dialog (icon actions on top, right-aligned). Every edit hits the
// live session jar immediately, so the next Send reflects it.
func (m *MainUI) showCookies() {
	if m.win == nil {
		return
	}
	jar := m.sess.CookieJar()
	cookies := jar.All()

	list := widget.NewList(
		func() int { return len(cookies) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(cookieSummary(cookies[i]))
		},
	)

	selectedIdx := -1
	var editBtn, deleteBtn *ttwidget.Button
	selectionChanged := func() {
		enableButton(editBtn, selectedIdx >= 0)
		enableButton(deleteBtn, selectedIdx >= 0)
	}
	list.OnSelected = func(i widget.ListItemID) { selectedIdx = i; selectionChanged() }
	list.OnUnselected = func(widget.ListItemID) { selectedIdx = -1; selectionChanged() }

	// reload re-snapshots the jar and resets the selection after any mutation.
	reload := func() {
		cookies = jar.All()
		selectedIdx = -1
		list.UnselectAll()
		list.Refresh()
		selectionChanged()
	}

	addBtn := tipButton("square-plus", "Add cookie", func() {
		// A manually-added cookie defaults to host-only (HostOnly: true), matching
		// a Set-Cookie with no Domain attribute — so it isn't silently replayed to
		// sub-domains unless the user opts in via "Send to subdomains".
		m.editCookie(cookiejar.Cookie{Path: "/", HostOnly: true}, func(c cookiejar.Cookie) {
			jar.Set(c)
			reload()
		})
	})
	editBtn = tipButton("pen-to-square", "Edit cookie", func() {
		if selectedIdx < 0 {
			return
		}
		orig := cookies[selectedIdx]
		m.editCookie(orig, func(c cookiejar.Cookie) {
			// Replace handles the (domain, path, name) identity: it drops the old
			// slot only when the identity changed (no orphan) and otherwise keeps
			// the cookie's send-order position for a value/flag-only edit.
			jar.Replace(orig.Domain, orig.Path, orig.Name, c)
			reload()
		})
	})
	deleteBtn = tipButton("trash-can", "Delete cookie", func() {
		if selectedIdx < 0 {
			return
		}
		c := cookies[selectedIdx]
		jar.Remove(c.Domain, c.Path, c.Name)
		reload()
	})
	clearBtn := tipButton("circle-xmark", "Clear all cookies", func() {
		if len(cookies) == 0 {
			return
		}
		dialog.ShowConfirm("Clear all cookies?",
			"Remove every cookie stored in this session's jar?",
			func(yes bool) {
				if yes {
					jar.Clear()
					reload()
				}
			}, m.win)
	})
	selectionChanged() // nothing selected yet → Edit/Delete start disabled

	actions := container.NewHBox(layout.NewSpacer(), addBtn, editBtn, deleteBtn, clearBtn)
	hint := widget.NewLabelWithStyle(
		"Cookies from Set-Cookie responses are stored for this session and replayed on matching requests.",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	content := container.NewBorder(actions, hint, nil, nil, list)
	d := dialog.NewCustom("Cookies", "Done", content, m.win)
	d.Resize(fyne.NewSize(620, 400))
	d.Show()
}

// editCookie shows a form to add or edit a single cookie. Domain and Name are
// required. The "Send to subdomains" check controls scope (HostOnly inverted);
// Expires is carried through unchanged since the form has no expiry input (so an
// edited persistent cookie keeps its expiry).
func (m *MainUI) editCookie(initial cookiejar.Cookie, onSave func(cookiejar.Cookie)) {
	domain := widget.NewEntry()
	domain.SetText(initial.Domain)
	domain.SetPlaceHolder("example.com")
	path := widget.NewEntry()
	path.SetText(initial.Path)
	path.SetPlaceHolder("/")
	name := widget.NewEntry()
	name.SetText(initial.Name)
	value := widget.NewEntry()
	value.SetText(initial.Value)
	secure := widget.NewCheck("Secure (https only)", nil)
	secure.SetChecked(initial.Secure)
	httpOnly := widget.NewCheck("HttpOnly", nil)
	httpOnly.SetChecked(initial.HTTPOnly)
	// HostOnly is the stored flag; the user-facing control is its inverse so the
	// label reads naturally. Unchecked ⇒ host-only (exact host); checked ⇒ also
	// sent to sub-domains.
	subdomains := widget.NewCheck("Send to subdomains", nil)
	subdomains.SetChecked(!initial.HostOnly)

	items := []*widget.FormItem{
		widget.NewFormItem("Domain", domain),
		widget.NewFormItem("Path", path),
		widget.NewFormItem("Name", name),
		widget.NewFormItem("Value", value),
		widget.NewFormItem("", secure),
		widget.NewFormItem("", httpOnly),
		widget.NewFormItem("", subdomains),
	}
	if !initial.Expires.IsZero() {
		items = append(items, widget.NewFormItem("Expires", widget.NewLabel(initial.Expires.Format(time.RFC1123))))
	}

	d := dialog.NewForm("Cookie", "Save", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		n, dm := strings.TrimSpace(name.Text), strings.TrimSpace(domain.Text)
		if n == "" || dm == "" {
			dialog.ShowError(errors.New("a cookie needs both a domain and a name"), m.win)
			return
		}
		onSave(cookiejar.Cookie{
			Name:     n,
			Value:    value.Text,
			Domain:   dm,
			Path:     strings.TrimSpace(path.Text),
			Expires:  initial.Expires,
			Secure:   secure.Checked,
			HTTPOnly: httpOnly.Checked,
			HostOnly: !subdomains.Checked,
		})
	}, m.win)
	d.Resize(fyne.NewSize(440, 360))
	d.Show()
}

// cookieSummary renders a one-line view of a cookie for the list: the scope,
// the name=value, and a bracketed flag list (path when non-root, host-only vs
// +subdomains, secure / httpOnly, and the session/expiry state).
func cookieSummary(c cookiejar.Cookie) string {
	s := fmt.Sprintf("%s  %s=%s", c.Domain, c.Name, c.Value)
	var flags []string
	if c.Path != "" && c.Path != "/" {
		flags = append(flags, "path "+c.Path)
	}
	if c.HostOnly {
		flags = append(flags, "host-only")
	} else {
		flags = append(flags, "+subdomains")
	}
	if c.Secure {
		flags = append(flags, "secure")
	}
	if c.HTTPOnly {
		flags = append(flags, "httpOnly")
	}
	if c.Expires.IsZero() {
		flags = append(flags, "session")
	} else {
		flags = append(flags, "expires "+c.Expires.Format("2006-01-02 15:04"))
	}
	return s + "  [" + strings.Join(flags, ", ") + "]"
}
