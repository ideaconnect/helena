package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// TestTopBarHelpTrailingAndOpens pins #129 (Settings/Help right-aligned) and the
// #128 button-type change: the Help button sits to the right of the workspace
// controls and its menu still opens when anchored to the icon button.
func TestTopBarHelpTrailingAndOpens(t *testing.T) {
	test.NewApp()
	s, _ := session.New("")
	m := NewMainUI(s)
	w := test.NewWindow(m.Root())
	w.Resize(fyne.NewSize(1100, 720))
	defer w.Close()
	m.SetWindow(w)
	w.Resize(fyne.NewSize(1100, 720))

	drv := fyne.CurrentApp().Driver()
	helpX := drv.AbsolutePositionForObject(m.helpBtn).X
	wsX := drv.AbsolutePositionForObject(m.Workspace).X
	if helpX <= wsX {
		t.Errorf("help button X=%v should be right of the workspace combo X=%v (#129)", helpX, wsX)
	}

	before := len(w.Canvas().Overlays().List())
	m.showHelpMenu()
	if len(w.Canvas().Overlays().List()) <= before {
		t.Error("showHelpMenu did not open a popup against the icon Help button (#128)")
	}
}
