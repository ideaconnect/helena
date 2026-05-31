package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/responsefmt"
)

// tokenColor maps a highlight token kind to its display color. The palette
// shares the method-chip saturation (see method.go) so the Pretty view and
// the Structured tree feel of-a-piece with the rest of the UI. TokenPlain
// falls through to the theme foreground.
func tokenColor(k responsefmt.TokenKind) color.Color {
	switch k {
	case responsefmt.TokenKey, responsefmt.TokenTag:
		return color.NRGBA{R: 0x8e, G: 0xc2, B: 0xec, A: 0xff} // blue
	case responsefmt.TokenString:
		return color.NRGBA{R: 0x53, G: 0xd0, B: 0x60, A: 0xff} // green
	case responsefmt.TokenNumber:
		return color.NRGBA{R: 0xe0, G: 0xa9, B: 0x7c, A: 0xff} // amber
	case responsefmt.TokenBool:
		return color.NRGBA{R: 0xc5, G: 0x69, B: 0x56, A: 0xff} // red
	case responsefmt.TokenAttr:
		return color.NRGBA{R: 0xe7, G: 0xae, B: 0x80, A: 0xff} // orange
	case responsefmt.TokenComment:
		return color.NRGBA{R: 0x7f, G: 0x9f, B: 0x7f, A: 0xff} // muted green
	case responsefmt.TokenNull, responsefmt.TokenPunct:
		return color.NRGBA{R: 0x9c, G: 0x9c, B: 0x9c, A: 0xff} // grey
	default:
		return theme.Color(theme.ColorNameForeground)
	}
}

// setColoredGrid renders a token stream into a TextGrid, one cell per rune,
// each cell foreground-colored by its token kind. A newline starts a new row.
// Passing nil tokens clears the grid.
func setColoredGrid(tg *widget.TextGrid, tokens []responsefmt.Token) {
	var rows []widget.TextGridRow
	cur := widget.TextGridRow{}
	for _, tok := range tokens {
		style := &widget.CustomTextGridStyle{FGColor: tokenColor(tok.Kind)}
		for _, r := range tok.Text {
			if r == '\n' {
				rows = append(rows, cur)
				cur = widget.TextGridRow{}
				continue
			}
			cur.Cells = append(cur.Cells, widget.TextGridCell{Rune: r, Style: style})
		}
	}
	rows = append(rows, cur)
	tg.Rows = rows
	tg.Refresh()
}

// buildStructuredTree constructs the Response "Structured" tab's tree. The
// tree reads from m.structRoot / m.structIndex, which showStructured swaps in
// after each response; an empty index renders a blank tree.
func (m *MainUI) buildStructuredTree() *widget.Tree {
	m.structIndex = map[string]*responsefmt.Node{}
	return widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID {
			if id == "" {
				if m.structRoot == nil {
					return nil
				}
				return []widget.TreeNodeID{m.structRoot.ID}
			}
			n := m.structIndex[id]
			if n == nil {
				return nil
			}
			ids := make([]widget.TreeNodeID, len(n.Children))
			for i, c := range n.Children {
				ids[i] = c.ID
			}
			return ids
		},
		func(id widget.TreeNodeID) bool {
			if id == "" {
				return true
			}
			n := m.structIndex[id]
			return n != nil && len(n.Children) > 0
		},
		func(bool) fyne.CanvasObject {
			label := canvas.NewText("", theme.Color(theme.ColorNameForeground))
			value := canvas.NewText("", theme.Color(theme.ColorNameForeground))
			return container.NewHBox(label, value)
		},
		func(id widget.TreeNodeID, _ bool, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			label := box.Objects[0].(*canvas.Text)
			value := box.Objects[1].(*canvas.Text)
			n := m.structIndex[id]
			if n == nil {
				label.Text, value.Text = "", ""
				label.Refresh()
				value.Refresh()
				return
			}
			label.Text = n.Label
			if n.Label != "" && n.Value != "" {
				label.Text = n.Label + ": "
			}
			label.Color = tokenColor(n.LabelKind)
			value.Text = n.Value
			value.Color = tokenColor(n.Kind)
			label.Refresh()
			value.Refresh()
		},
	)
}

// renderResponseBody fills the Structured and Pretty tabs from a response body
// and content type, then selects the richest tab that succeeded: Structured if
// the body parsed into a tree, else Pretty if it formatted + colored, else Raw.
// JSON and XML are the only structured types; anything else clears both rich
// views and leaves the user on Raw.
func (m *MainUI) renderResponseBody(body []byte, contentType string) {
	var haveStruct, havePretty bool
	switch {
	case responsefmt.IsJSON(contentType):
		if p, err := responsefmt.PrettyJSON(body); err == nil {
			setColoredGrid(m.prettyGrid, responsefmt.HighlightJSON(p))
			havePretty = true
		}
		if root, err := responsefmt.ParseJSON(body); err == nil {
			m.showStructured(root)
			haveStruct = true
		}
	case responsefmt.IsXML(contentType):
		if p, err := responsefmt.PrettyXML(body); err == nil {
			setColoredGrid(m.prettyGrid, responsefmt.HighlightXML(p))
			havePretty = true
		}
		if root, err := responsefmt.ParseXML(body); err == nil {
			m.showStructured(root)
			haveStruct = true
		}
	}
	if !havePretty {
		setColoredGrid(m.prettyGrid, nil)
	}
	if !haveStruct {
		m.showStructured(nil)
	}
	switch {
	case haveStruct:
		m.Response.SelectIndex(0)
	case havePretty:
		m.Response.SelectIndex(1)
	default:
		m.Response.SelectIndex(2)
	}
}

// clearRichResponse blanks the Structured and Pretty tabs — used on error
// paths that put the failure text on the Raw tab instead.
func (m *MainUI) clearRichResponse() {
	setColoredGrid(m.prettyGrid, nil)
	m.showStructured(nil)
}

// showStructured swaps in a freshly parsed tree (or nil to clear), rebuilds
// the ID index the tree callbacks read, and expands the root so the first
// level is visible without a click.
func (m *MainUI) showStructured(root *responsefmt.Node) {
	m.structRoot = root
	m.structIndex = map[string]*responsefmt.Node{}
	if root != nil {
		indexStructured(m.structIndex, root)
	}
	m.structuredTree.Refresh()
	if root != nil {
		m.structuredTree.OpenBranch(root.ID)
	}
}

// indexStructured records every node by ID so the tree's childUIDs/isBranch
// callbacks resolve in O(1).
func indexStructured(idx map[string]*responsefmt.Node, n *responsefmt.Node) {
	idx[n.ID] = n
	for _, c := range n.Children {
		indexStructured(idx, c)
	}
}
