package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/model"
)

// sanitizeTimeoutSeconds parses a user-entered request-timeout field. A blank,
// non-numeric, or non-positive entry is rejected (valid=false) and the supplied
// fallback is returned instead of silently becoming 0 — which means an
// UNLIMITED timeout and is the same unsafe zero-value the config defaults guard
// against. Helena exposes no explicit "unlimited" control, so a parse failure
// must never produce one. The fallback (the user's current timeout) is itself
// floored to the default when non-positive, so the result is always > 0.
func sanitizeTimeoutSeconds(text string, fallback int) (seconds int, valid bool) {
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n > 0 {
		return n, true
	}
	if fallback > 0 {
		return fallback, false
	}
	return model.DefaultSettings().TimeoutSeconds, false
}

// sanitizeMaxResponseMiB parses the response-body cap field (entered in MiB)
// into bytes. A blank, non-numeric, or non-positive entry is rejected
// (valid=false) and the supplied fallback (bytes) is returned, floored to the
// built-in default so the result is always a safe positive bound.
func sanitizeMaxResponseMiB(text string, fallback int64) (bytes int64, valid bool) {
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n > 0 {
		return int64(n) << 20, true
	}
	if fallback > 0 {
		return fallback, false
	}
	return model.DefaultMaxResponseBytes, false
}

// editSettings opens the Theme / SSL / CORS / redirects / timeout dialog and
// persists changes via the session.
func (m *MainUI) editSettings() {
	if m.win == nil {
		return
	}
	s := m.sess.Settings()

	insecure := widget.NewCheck("Allow invalid / self-signed TLS certificates", nil)
	insecure.SetChecked(s.InsecureSkipVerify)
	// Spell out the blast radius: the toggle disables cert validation for every
	// request AND the spec importer, not just one dev endpoint (#45).
	insecureCaption := widget.NewLabel("Disables certificate validation for ALL requests and spec imports — use only for trusted dev endpoints.")
	insecureCaption.Wrapping = fyne.TextWrapWord
	insecureCaption.Importance = widget.WarningImportance
	insecureBox := container.NewVBox(insecure, insecureCaption)
	corsWarn := widget.NewCheck("Warn when a browser would block the response (CORS)", nil)
	corsWarn.SetChecked(s.CORSWarning)
	follow := widget.NewCheck("Follow redirects automatically", nil)
	follow.SetChecked(s.FollowRedirects)
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText(strconv.Itoa(s.TimeoutSeconds))
	timeoutEntry.SetPlaceHolder("0 = no timeout")

	curMax := s.MaxResponseBytes
	if curMax <= 0 {
		curMax = model.DefaultMaxResponseBytes
	}
	maxRespEntry := widget.NewEntry()
	maxRespEntry.SetText(strconv.FormatInt(curMax>>20, 10))
	maxRespEntry.SetPlaceHolder("buffered response cap")

	themeSelect := widget.NewSelect([]string{"System", "Light", "Dark"}, nil)
	themeSelect.SetSelected(themeName(s.Theme))

	// Global variables (#83) are app-wide and persisted in the config, so they
	// live in Settings; the button opens the shared variables editor.
	globalVarsBtn := widget.NewButtonWithIcon("Edit global variables…", themedIcon("table-list"), m.editGlobalVariables)

	form := widget.NewForm(
		widget.NewFormItem("Theme", themeSelect),
		widget.NewFormItem("TLS", insecureBox),
		widget.NewFormItem("CORS", corsWarn),
		widget.NewFormItem("Redirects", follow),
		widget.NewFormItem("Timeout (s)", timeoutEntry),
		widget.NewFormItem("Max response (MiB)", maxRespEntry),
		widget.NewFormItem("Global variables", globalVarsBtn),
	)

	dlg := dialog.NewCustomConfirm("Settings", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		m.guard("Save settings", func() {
			t, validTimeout := sanitizeTimeoutSeconds(timeoutEntry.Text, m.sess.Settings().TimeoutSeconds)
			maxResp, validMax := sanitizeMaxResponseMiB(maxRespEntry.Text, m.sess.Settings().MaxResponseBytes)
			newTheme := themeFromName(themeSelect.Selected)
			m.sess.SetSettings(model.Settings{
				InsecureSkipVerify: insecure.Checked,
				CORSWarning:        corsWarn.Checked,
				FollowRedirects:    follow.Checked,
				TimeoutSeconds:     t,
				MaxResponseBytes:   maxResp,
				Theme:              newTheme,
			})
			ApplyTheme(fyne.CurrentApp(), newTheme)
			// PrettyView memoizes its palette on the Fyne theme variant, which
			// ApplyTheme's SetTheme(Light/Dark) does not flip — so force a rebuild
			// at the new variant, or the body keeps the old light/dark colors.
			m.pv.SetTheme(variantFor(newTheme), prettyview.Theme{})
			m.BodyContent.SetTheme(variantFor(newTheme), prettyview.Theme{})
			m.refreshThemedCanvas()
			switch {
			case validTimeout && validMax:
				m.Status.SetText("Settings saved")
			case !validTimeout && !validMax:
				m.Status.SetText(fmt.Sprintf("Settings saved (invalid timeout kept %ds, invalid max response kept %d MiB)", t, maxResp>>20))
			case !validTimeout:
				m.Status.SetText(fmt.Sprintf("Settings saved (invalid timeout ignored, kept %ds)", t))
			default:
				m.Status.SetText(fmt.Sprintf("Settings saved (invalid max response ignored, kept %d MiB)", maxResp>>20))
			}
		})
	}, m.win)
	dlg.Resize(fyne.NewSize(520, 420))
	dlg.Show()
}

// refreshThemedCanvas re-tints the raw canvas objects whose colours are set
// imperatively rather than read from the theme on every paint. Fyne's
// SetTheme repaints theme-driven widgets but not bare canvas.Text /
// canvas.Rectangle, so without this a Light↔Dark switch leaves the CORS
// banner (and the active-tab highlight) showing the old variant's colour.
// Method chips use fixed brand colours (see method.go) so they need no
// re-tint. Called from editSettings after the theme swap.
func (m *MainUI) refreshThemedCanvas() {
	if m.corsBanner != nil {
		m.corsBanner.Color = theme.Color(theme.ColorNameWarning)
		m.corsBanner.Refresh()
	}
	if m.tabStripBg != nil {
		m.tabStripBg.FillColor = theme.Color(theme.ColorNameHeaderBackground)
		m.tabStripBg.Refresh()
	}
	m.rebuildTabBar() // repaints each tab's selection-coloured highlight
	if m.Tree != nil {
		m.Tree.Refresh()
	}
}
