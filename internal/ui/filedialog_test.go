package ui

import (
	"testing"

	"fyne.io/fyne/v2"
)

// TestFileDialogSize verifies the picker sizing: a usable floor when the canvas
// size is unknown, ~85% of a large window, and a clamp to the window when it is
// smaller than the floor.
func TestFileDialogSize(t *testing.T) {
	const eps = float32(1)
	cases := []struct {
		name   string
		canvas fyne.Size
		wantW  float32
		wantH  float32
	}{
		{"unknown-canvas", fyne.NewSize(0, 0), 820, 600},
		{"large-window-85pct", fyne.NewSize(1100, 720), 935, 612},
		{"small-window-clamped", fyne.NewSize(600, 400), 600, 400},
		{"floor-sized-window", fyne.NewSize(820, 600), 820, 600},
		{"wide-short-window", fyne.NewSize(2000, 500), 1700, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fileDialogSize(tc.canvas)
			if abs(got.Width-tc.wantW) > eps || abs(got.Height-tc.wantH) > eps {
				t.Errorf("fileDialogSize(%v) = %v, want ~%gx%g", tc.canvas, got, tc.wantW, tc.wantH)
			}
			// Never exceed a known window.
			if tc.canvas.Width > 0 && (got.Width > tc.canvas.Width+eps || got.Height > tc.canvas.Height+eps) {
				t.Errorf("size %v exceeds canvas %v", got, tc.canvas)
			}
		})
	}
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
