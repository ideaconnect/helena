package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// scriptConsoleMaxLines caps how many script-emitted console lines the
// panel renders at once. Noisy scripts (`for(let i=0;i<1e6;i++) ...`)
// would otherwise hand a multi-megabyte string to a Fyne text widget
// on the UI goroutine. When the cap kicks in, we keep the most recent
// lines and prepend a "[truncated, kept last N]" notice so the user
// knows output was dropped.
const scriptConsoleMaxLines = 500

// buildScriptsTab assembles the request-editor's Scripts tab: two
// subtab-hosted monospace editors (Pre-request, Post-response) plus a
// shared read-only Console panel that shows whatever the last Send's
// hooks emitted via console.log / info / error / warn.
//
// Both editor OnChanged callbacks are guarded by m.loading so the
// programmatic SetText calls in loadRequest don't echo back into the
// model the way every other request widget is wired.
func (m *MainUI) buildScriptsTab() fyne.CanvasObject {
	m.preScriptEditor = m.newShortcutMultiLineEntry()
	m.preScriptEditor.Wrapping = fyne.TextWrapOff
	m.preScriptEditor.TextStyle = fyne.TextStyle{Monospace: true}
	m.preScriptEditor.SetPlaceHolder("// Runs before the request goes out.\n" +
		"// request.method / url / body / headers / params are mutable.\n" +
		"// helena.env.set(name, value) writes to the session-only overlay.")
	m.preScriptEditor.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Scripts.PreRequest = s
		}
	}

	m.postScriptEditor = m.newShortcutMultiLineEntry()
	m.postScriptEditor.Wrapping = fyne.TextWrapOff
	m.postScriptEditor.TextStyle = fyne.TextStyle{Monospace: true}
	m.postScriptEditor.SetPlaceHolder("// Runs after the response is read.\n" +
		"// response.status / statusText / headers / body / text / json / xml are read-only.\n" +
		"// Example: helena.env.set(\"TOKEN\", response.json.token)")
	m.postScriptEditor.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Scripts.PostResponse = s
		}
	}

	preTab := container.NewTabItem("Pre-request", container.NewVScroll(m.preScriptEditor))
	postTab := container.NewTabItem("Post-response", container.NewVScroll(m.postScriptEditor))
	editors := container.NewAppTabs(preTab, postTab)

	m.scriptConsole = m.newShortcutMultiLineEntry()
	m.scriptConsole.Wrapping = fyne.TextWrapOff
	m.scriptConsole.TextStyle = fyne.TextStyle{Monospace: true}
	m.scriptConsole.SetPlaceHolder("Console output from the last Send appears here.")
	m.scriptConsole.SetMinRowsVisible(4)
	m.scriptConsole.Disable() // read-only; SetText still works on the widget.
	consoleLabel := widget.NewLabelWithStyle("Console", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	clearBtn := widget.NewButton("Clear", func() { m.setScriptConsole(nil) })
	consoleHeader := container.NewBorder(nil, nil, consoleLabel, clearBtn, nil)
	consolePanel := container.NewBorder(consoleHeader, nil, nil, nil, container.NewVScroll(m.scriptConsole))

	split := container.NewVSplit(editors, consolePanel)
	split.SetOffset(0.7)
	return split
}

// loadScriptsTab pushes req.Scripts into the two editors. Called from
// loadRequest under the m.loading guard so OnChanged is a no-op during
// the bulk-update. Nil req clears both editors and the console.
func (m *MainUI) loadScriptsTab(req *model.Request) {
	if m.preScriptEditor == nil || m.postScriptEditor == nil {
		return
	}
	if req == nil {
		m.preScriptEditor.SetText("")
		m.postScriptEditor.SetText("")
		m.setScriptConsole(nil)
		return
	}
	m.preScriptEditor.SetText(req.Scripts.PreRequest)
	m.postScriptEditor.SetText(req.Scripts.PostResponse)
}

// setScriptConsole renders lines (oldest first) into the read-only
// Console panel. Called from m.send once the pre/post results come
// back so the user can see what their scripts printed during the last
// Send. Safe to call before the panel is built — no-ops in that case.
// Output exceeding scriptConsoleMaxLines is truncated to the most
// recent lines with a notice on the first line so a noisy script can't
// freeze the Fyne text widget with a multi-megabyte SetText.
func (m *MainUI) setScriptConsole(lines []string) {
	if m.scriptConsole == nil {
		return
	}
	if len(lines) > scriptConsoleMaxLines {
		dropped := len(lines) - scriptConsoleMaxLines
		notice := fmt.Sprintf("[truncated %d earlier lines]", dropped)
		lines = append([]string{notice}, lines[dropped:]...)
	}
	m.scriptConsole.SetText(strings.Join(lines, "\n"))
}
