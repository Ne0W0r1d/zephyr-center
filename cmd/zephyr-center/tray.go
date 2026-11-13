// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package main

import (
	"log"
	"sync/atomic"

	"github.com/egoist/mygo"

	"github.com/Ne0W0r1d/zephyr-center/internal/app"
	"github.com/Ne0W0r1d/zephyr-center/internal/appicon"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// trayHandle is the platform's notification-area icon. Linux registers a
// StatusNotifierItem over D-Bus itself; Windows and macOS use MyGo's
// native notification area.
type trayHandle interface {
	SetToolTip(string)
	Destroy()
}

// trayCallbacks are the actions the tray exposes. Activate receives the
// click position in screen coordinates when the platform knows it, and
// zeroes otherwise.
type trayCallbacks struct {
	Activate func(x, y int)
	OpenMain func()
	Quit     func()
}

// trayApp keeps Zephyr Center reachable while its window is hidden and
// owns the actions behind the notification-area icon.
type trayApp struct {
	win      *mygo.Window
	quick    *quickPanel
	tray     trayHandle
	quitting atomic.Bool
}

// attachTray adds the notification-area icon and a quick panel, and makes
// the window's close button hide it instead of quitting. Without a working
// tray the close button keeps its normal meaning, so the app cannot
// disappear with no way to bring it back.
func attachTray(win *mygo.Window, a *app.App) *trayApp {
	t := &trayApp{win: win}
	if err := win.SetIcon(appicon.AppPNG); err != nil {
		log.Printf("zephyr-center: window icon unavailable: %v", err)
	}

	locale := mygo.App.Locale()
	t.quick = newQuickPanel(a, func() bool { return t.quitting.Load() }, t.show)
	handle, err := newPlatformTray(appicon.TrayPNG, i18n.TLocale(locale, "app.title"), trayCallbacks{
		Activate: t.quick.ToggleAt,
		OpenMain: t.show,
		Quit:     t.quit,
	})
	if err != nil {
		log.Printf("zephyr-center: tray unavailable: %v", err)
		t.quick.destroy()
		t.quick = nil
		return t
	}
	t.tray = handle

	win.OnClose(func(e *mygo.CloseEvent) {
		if t.quitting.Load() {
			return
		}
		e.PreventDefault()
		win.Hide()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		t.quitting.Store(true)
		if t.tray != nil {
			t.tray.Destroy()
		}
	})
	mygo.App.OnLocaleChanged(func(locale string) {
		handle.SetToolTip(i18n.TLocale(locale, "app.title"))
	})
	return t
}

// show brings the window back from the tray and puts the quick panel
// away first, restoring the window if the user had minimized it.
func (t *trayApp) show() {
	if t.quick != nil {
		t.quick.hide()
	}
	if t.win.IsMinimized() {
		t.win.Restore()
	}
	t.win.Show()
	t.win.Focus()
}

// quit lets the close handler run and ends the event loop.
func (t *trayApp) quit() {
	t.quitting.Store(true)
	mygo.App.Quit()
}

// The quick panel is a launcher-style panel; the window manager may place
// it itself.
const (
	quickPanelWidth  = 400
	quickPanelHeight = 560
	quickPanelGap    = 12
)
