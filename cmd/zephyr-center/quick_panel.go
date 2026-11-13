// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package main

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/app"
)

// quickPanel is the frameless panel the tray icon opens. It is a MyGo
// panel: it floats above other windows and stays out of the taskbar.
type quickPanel struct {
	win        *mygo.Window
	quitting   func() bool
	destroying atomic.Bool
}

// newQuickPanel creates the hidden panel window. Callbacks are kept as
// closures so the panel itself does not need to know about the tray.
func newQuickPanel(a *app.App, quitting func() bool, openMain func()) *quickPanel {
	q := &quickPanel{quitting: quitting}
	q.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Zephyr Center",
		Width:             quickPanelWidth,
		Height:            quickPanelHeight,
		Panel:             true,
		Hidden:            true,
		Frameless:         true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		SkipTaskbar:       true,
		AlwaysOnTop:       true,
		CornerRadius:      16,
		BackgroundColor:   "light-dark(#f7f6f3, #14171a)",
		Content:           ui.View(a.QuickView(openMain, q.hide, quitApp)),
	})
	q.win.OnClose(func(e *mygo.CloseEvent) {
		if q.destroying.Load() || (q.quitting != nil && q.quitting()) {
			return
		}
		e.PreventDefault()
		q.hide()
	})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if q.destroying.Load() {
				return
			}
			q.win.Invalidate()
		}
	}()
	return q
}

// ToggleAt opens the panel at a tray click, or closes it when already
// open. A zero position means the platform did not report where the icon
// is; the pointer's position is the best available anchor.
func (q *quickPanel) ToggleAt(x, y int) {
	if q.win.IsVisible() {
		q.hide()
		return
	}
	q.showAt(x, y)
}

// showAt places the panel near the click and shows it. It stays inside
// the work area of the display under the pointer.
func (q *quickPanel) showAt(x, y int) {
	if x == 0 && y == 0 {
		p := mygo.Screen.CursorScreenPoint()
		x, y = p.X, p.Y
	}
	width, height := q.win.Size()
	display := mygo.Screen.DisplayNearestPoint(mygo.Point{X: x, Y: y})
	area := display.WorkArea

	px := x - width + 24
	py := y - height - quickPanelGap
	if y < area.Y+area.Height/2 {
		// A tray on the top edge (macOS, some Linux panels) opens below.
		py = y + quickPanelGap
	}
	px = clampInt(px, area.X, area.X+area.Width-width)
	py = clampInt(py, area.Y, area.Y+area.Height-height)

	// GDK reports X11 monitor geometry in device pixels under some
	// XWayland scale setups while gtk_window_move takes application
	// pixels. Requesting a position then sends the panel off the screen.
	// Let the window manager place it instead on that path; compositors
	// place a normal panel themselves anyway.
	if runtime.GOOS != "linux" || display.ScaleFactor <= 1 {
		q.win.SetPosition(px, py)
	}
	q.win.Show()
	q.win.Focus()
}

// hide puts the panel away without destroying it, so opening it again is
// immediate.
func (q *quickPanel) hide() {
	q.win.Hide()
}

// destroy closes the hidden window when the tray itself is unavailable.
// The close handler would otherwise hide it forever.
func (q *quickPanel) destroy() {
	q.destroying.Store(true)
	q.win.Close()
}

// quitApp exits through MyGo so OnBeforeQuit runs and the tray's close
// handler allows the windows to close.
func quitApp() { mygo.App.Quit() }

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
