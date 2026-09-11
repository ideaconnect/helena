package ui

import (
	"fyne.io/fyne/v2"

	"github.com/idct/helena/internal/model"
)

// buildAuthTab assembles the per-request Auth tab: an authEditor bound to the
// current request's Auth. Write-back honours m.loading (via the editor's busy
// hook) and marks the active tab dirty. The Inherit panel previews what the
// request would receive from its folder → collection chain.
func (m *MainUI) buildAuthTab() fyne.CanvasObject {
	m.authEd = newAuthEditor(m, authEditorOpts{
		target: func() *model.Auth {
			if m.currentRequest == nil {
				return nil
			}
			return &m.currentRequest.Auth
		},
		onChange:       m.refreshActiveTabDirty,
		inheritText:    m.requestInheritText,
		busy:           func() bool { return m.loading },
		allowInherit:   true,
		tokenNamespace: m.sess.ActiveCollectionDir,
	})
	return m.authEd.root
}

// requestInheritText is the Auth tab's Inherit-panel line: the auth the
// current request would inherit from its ancestors (Session.InheritedAuth —
// deliberately NOT EffectiveAuth, which would echo the request's own saved
// auth back while the user is switching it to Inherit).
func (m *MainUI) requestInheritText() string {
	if m.currentRequestID == "" {
		return "Inheriting — no resolved auth available (no request selected)."
	}
	return inheritPreview(m.sess.InheritedAuth(m.currentRequestID))
}

// refreshAuthInheritLabel re-computes the Auth tab's Inherit preview. Called
// after loading a request and after folder / collection auth changes so an
// open Inherit request reflects the new ancestor.
func (m *MainUI) refreshAuthInheritLabel() {
	if m.authEd == nil {
		return
	}
	m.authEd.refreshInheritLabel()
}

// loadAuthTab populates every auth widget from req.Auth without firing
// write-back callbacks. Called from loadRequest under the m.loading guard; a
// nil req resets the tab to a blank Inherit form.
func (m *MainUI) loadAuthTab(req *model.Request) {
	if m.authEd == nil {
		return
	}
	if req == nil {
		m.authEd.load(model.Auth{Type: model.AuthInherit})
		return
	}
	m.authEd.load(req.Auth)
}
