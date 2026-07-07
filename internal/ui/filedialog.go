package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// fileDialogSize returns a comfortable size for a file/folder picker given the
// parent canvas size. Fyne's default file dialog opens small for its large file
// icons, leaving only a couple of items visible — so we open it at ~85% of the
// window, but never below a usable floor and never larger than the window.
func fileDialogSize(canvas fyne.Size) fyne.Size {
	w, h := float32(820), float32(600)
	if canvas.Width > 0 && canvas.Height > 0 {
		if p := canvas.Width * 0.85; p > w {
			w = p
		}
		if p := canvas.Height * 0.85; p > h {
			h = p
		}
		if w > canvas.Width {
			w = canvas.Width
		}
		if h > canvas.Height {
			h = canvas.Height
		}
	}
	return fyne.NewSize(w, h)
}

// showFileDialog enlarges a file/folder picker to fileDialogSize and shows it, so
// every picker in the app opens roomier than Fyne's cramped default. All open /
// save / folder pickers route through here.
func (m *MainUI) showFileDialog(d *dialog.FileDialog) {
	var cs fyne.Size
	if m.win != nil {
		cs = m.win.Canvas().Size()
	}
	d.Resize(fileDialogSize(cs))
	d.Show()
}
