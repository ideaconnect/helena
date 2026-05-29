package ui

import (
	"net/url"

	"fyne.io/fyne/v2"

	"github.com/idct/helena/internal/auth"
)

// fyneAuthCodeStarter implements auth.AuthCodeStarter by delegating to
// fyne.CurrentApp().OpenURL — the only sanctioned way to launch the
// user's default browser from a Fyne application. The redirect listener,
// state generation, code exchange, and PKCE all live in internal/auth;
// this adapter exists purely to keep that package free of Fyne imports
// so it stays unit-testable without the GUI toolkit.
type fyneAuthCodeStarter struct{}

// OpenAuthURL parses authURL and hands it to the running Fyne app.
// Errors from url.Parse or OpenURL bubble up so the resolver can fail
// fast instead of waiting out the 5-minute callback timeout.
func (fyneAuthCodeStarter) OpenAuthURL(authURL string) error {
	u, err := url.Parse(authURL)
	if err != nil {
		return err
	}
	return fyne.CurrentApp().OpenURL(u)
}

// newAuthCodeStarter returns the Fyne-backed AuthCodeStarter used at
// runtime. Tests can substitute a different value by setting this hook.
var newAuthCodeStarter = func() auth.AuthCodeStarter { return fyneAuthCodeStarter{} }
