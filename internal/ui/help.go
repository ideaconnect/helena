package ui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	repoURL      = "https://github.com/ideaconnect/helena"
	userGuideURL = repoURL + "/blob/main/docs/USER_GUIDE.md"
	issuesURL    = repoURL + "/issues"
)

// SetVersion records the build version for the Help → About entry (#61). main
// calls it after NewMainUI so the UI package needn't import the build vars.
func (m *MainUI) SetVersion(v string) { m.appVersion = v }

// helpMenuItems builds the Help menu: a getting-started pointer, the keymap,
// web links to the guide and issue tracker, and About — more than just the
// shortcuts list (#61).
func (m *MainUI) helpMenuItems() []*fyne.MenuItem {
	return []*fyne.MenuItem{
		fyne.NewMenuItem("Getting started", m.showGettingStarted),
		fyne.NewMenuItem("Keyboard shortcuts", m.showShortcuts),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("User guide (web)", func() { m.openURL(userGuideURL) }),
		fyne.NewMenuItem("Report an issue (web)", func() { m.openURL(issuesURL) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("About Helena", m.showAbout),
	}
}

// showHelpMenu pops up the Help menu anchored under the "?" toolbar button.
func (m *MainUI) showHelpMenu() {
	if m.win == nil || m.helpBtn == nil {
		return
	}
	canv := m.win.Canvas()
	popUp := widget.NewPopUpMenu(fyne.NewMenu("Help", m.helpMenuItems()...), canv)
	if d := fyne.CurrentApp().Driver(); d != nil {
		pos := d.AbsolutePositionForObject(m.helpBtn)
		popUp.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+m.helpBtn.Size().Height))
		return
	}
	popUp.Show()
}

// openURL opens raw in the user's browser via the Fyne app, surfacing any
// error rather than failing silently.
func (m *MainUI) openURL(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		if m.win != nil {
			dialog.ShowError(err, m.win)
		}
		return
	}
	if err := fyne.CurrentApp().OpenURL(u); err != nil && m.win != nil {
		dialog.ShowError(err, m.win)
	}
}

// showGettingStarted shows a brief first-steps overview.
func (m *MainUI) showGettingStarted() {
	if m.win == nil {
		return
	}
	const body = "Getting started with Helena\n\n" +
		"1. Add a collection: Open an existing OpenCollection folder, Import an\n" +
		"   OpenAPI/Swagger/WSDL spec, or Load the bundled sample.\n" +
		"2. Pick a request in the sidebar, set the method + URL, and edit the\n" +
		"   Query / Headers / Body / Auth tabs.\n" +
		"3. Define {{variables}} per environment (Variables…); switch the active\n" +
		"   environment from the dropdown.\n" +
		"4. Send (Mod+Enter). The response shows body, headers, timing, and any\n" +
		"   script console output.\n\n" +
		"See the User guide (Help → User guide) for chaining, scripting, and more."
	dialog.ShowInformation("Getting started", body, m.win)
}

// showAbout shows the app name, version (when set), and repo link.
func (m *MainUI) showAbout() {
	if m.win == nil {
		return
	}
	v := m.appVersion
	if v == "" {
		v = "dev"
	}
	dialog.ShowInformation("About Helena",
		"Helena — a free, open-source, devs-for-devs API client.\n\n"+
			"Version: "+v+"\n"+
			repoURL,
		m.win)
}
