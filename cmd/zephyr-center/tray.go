// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package main

import (
	"log"
	"sync/atomic"

	"github.com/egoist/mygo"

	"github.com/Ne0W0r1d/zephyr-center/internal/appicon"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// trayApp keeps Zephyr Center reachable while its window is hidden and
// owns the actions behind the notification-area icon.
type trayApp struct {
	win      *mygo.Window
	tray     *mygo.Tray
	quitting atomic.Bool
}

// attachTray adds the notification-area icon and makes the window's close
// button hide it instead of quitting. Without a working tray the close
// button keeps its normal meaning, so the app cannot disappear with no way
// to bring it back.
func attachTray(win *mygo.Window) *trayApp {
	t := &trayApp{win: win}
	if err := win.SetIcon(appicon.AppPNG); err != nil {
		log.Printf("zephyr-center: window icon unavailable: %v", err)
	}

	locale := mygo.App.Locale()
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "zephyr-center",
		Icon:    appicon.TrayPNG,
		ToolTip: i18n.TLocale(locale, "app.title"),
		Menu:    t.menu(locale),
	})
	if err != nil {
		log.Printf("zephyr-center: tray unavailable: %v", err)
		return t
	}
	t.tray = tray

	win.OnClose(func(e *mygo.CloseEvent) {
		if t.quitting.Load() {
			return
		}
		e.PreventDefault()
		win.Hide()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		t.quitting.Store(true)
	})
	mygo.App.OnLocaleChanged(func(locale string) {
		tray.SetMenu(t.menu(locale))
		tray.SetToolTip(i18n.TLocale(locale, "app.title"))
	})
	return t
}

// menu builds the tray menu in locale. It is rebuilt when the app's
// language changes.
func (t *trayApp) menu(locale string) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: i18n.TLocale(locale, "tray.open"), Click: func(*mygo.MenuItem, *mygo.Window) {
			t.show()
		}},
		mygo.Separator(),
		{Label: i18n.TLocale(locale, "tray.quit"), Click: func(*mygo.MenuItem, *mygo.Window) {
			t.quit()
		}},
	})
}

// show brings the window back from the tray, restoring it first if the
// user had minimized it.
func (t *trayApp) show() {
	if t.win.IsMinimized() {
		t.win.Restore()
	}
	t.win.Show()
	t.win.Focus()
}

// quit lets the close handler run and ends the event loop.
func (t *trayApp) quit() { mygo.App.Quit() }
