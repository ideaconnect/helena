package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// buildDocsTab assembles the request-editor's Docs tab: a monospace
// MultiLineEntry for raw Markdown plus a Preview subtab rendered via
// widget.RichText. The preview re-parses the editor's current text whenever
// the user switches to it — cheap enough not to need debouncing, and avoids
// the half-rendered intermediate states a live preview would show.
func (m *MainUI) buildDocsTab() *container.AppTabs {
	m.docsEditor = widget.NewMultiLineEntry()
	m.docsEditor.Wrapping = fyne.TextWrapOff
	m.docsEditor.TextStyle = fyne.TextStyle{Monospace: true}
	m.docsEditor.SetPlaceHolder("Markdown notes for this request — usage, edge cases, links, examples.\n" +
		"Switch to the Preview subtab to see it rendered.")
	m.docsEditor.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Docs = s
		}
	}

	m.docsPreview = widget.NewRichTextFromMarkdown("")
	m.docsPreview.Wrapping = fyne.TextWrapWord

	editTab := container.NewTabItem("Edit", container.NewVScroll(m.docsEditor))
	previewTab := container.NewTabItem("Preview", container.NewVScroll(m.docsPreview))

	docsTabs := container.NewAppTabs(editTab, previewTab)
	docsTabs.OnSelected = func(ti *container.TabItem) {
		if ti == previewTab {
			m.refreshDocsPreview()
		}
	}
	return docsTabs
}

// refreshDocsPreview re-parses the current docsEditor text and pushes it
// into the RichText widget. Safe to call when widgets aren't built yet.
func (m *MainUI) refreshDocsPreview() {
	if m.docsPreview == nil || m.docsEditor == nil {
		return
	}
	m.docsPreview.ParseMarkdown(m.docsEditor.Text)
}
