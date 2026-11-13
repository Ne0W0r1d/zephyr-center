// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build !linux

package main

import (
	"github.com/egoist/mygo"

	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// nativeTray is MyGo's notification-area icon on Windows and macOS. Linux
// uses the StatusNotifierItem implementation in tray_linux.go instead, so
// it does not need libayatana-appindicator.
type nativeTray struct {
	tray *mygo.Tray
	menu *mygo.Menu
}

func newPlatformTray(icon []byte, tooltip string, cb trayCallbacks) (trayHandle, error) {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "zephyr-center",
		Icon:    icon,
		ToolTip: tooltip,
	})
	if err != nil {
		return nil, err
	}
	t := &nativeTray{tray: tray}
	t.menu = t.buildMenu(cb)
	tray.OnClick(func() { cb.Activate(0, 0) })
	tray.OnRightClick(func() {
		tray.SetMenu(t.menu)
		defer tray.SetMenu(nil)
		tray.PopUpMenu()
	})
	return t, nil
}

func (t *nativeTray) buildMenu(cb trayCallbacks) *mygo.Menu {
	locale := mygo.App.Locale()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: i18n.TLocale(locale, "tray.open"), Click: func(*mygo.MenuItem, *mygo.Window) {
			if cb.OpenMain != nil {
				cb.OpenMain()
			}
		}},
		mygo.Separator(),
		{Label: i18n.TLocale(locale, "tray.quit"), Click: func(*mygo.MenuItem, *mygo.Window) {
			if cb.Quit != nil {
				cb.Quit()
			}
		}},
	})
}

func (t *nativeTray) SetToolTip(tip string) { t.tray.SetToolTip(tip) }
func (t *nativeTray) Destroy()              { t.tray.Destroy() }
