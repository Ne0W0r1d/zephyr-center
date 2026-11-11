// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/sound"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// windowControl is the part of mygo.Window the client-side title bar
// drives. The app keeps it as an interface so tests can use a stand-in.
type windowControl interface {
	Minimize()
	ToggleMaximize()
	Close()
	IsMaximized() bool
}

// AttachWindow gives the view the window it draws the title bar for.
func (a *App) AttachWindow(w windowControl) {
	a.mu.Lock()
	a.window = w
	a.mu.Unlock()
}

// windowActions returns the attached window, or nil before NewWindow.
func (a *App) windowActions() windowControl {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.window
}

// windowButtons draws the minimize, maximize and close controls. The
// system's own buttons are hidden on every platform, so the same chrome
// shows up whether the window runs on Linux or Windows.
func (a *App) windowButtons(c *ui.Context) {
	w := a.windowActions()
	maximized := w != nil && w.IsMaximized()

	ui.Row(c).Gap(0).AlignItems(ui.Center).Children(func() {
		a.windowButton(c, i18n.T(c, "app.minimize"), "minus", false, func() {
			if w := a.windowActions(); w != nil {
				w.Minimize()
			}
		})

		maxLabel, maxGlyph := i18n.T(c, "app.maximize"), "square"
		if maximized {
			maxLabel, maxGlyph = i18n.T(c, "app.restore"), "copy"
		}
		a.windowButton(c, maxLabel, maxGlyph, false, func() {
			if w := a.windowActions(); w != nil {
				w.ToggleMaximize()
			}
		}).MaximizeButton()

		a.windowButton(c, i18n.T(c, "app.close"), "x", true, func() {
			if w := a.windowActions(); w != nil {
				w.Close()
			}
		})
	})
}

// windowButton is one title bar control. It keeps the pointer on itself,
// so the title bar's drag area does not swallow clicks.
func (a *App) windowButton(c *ui.Context, label, glyph string, danger bool, fn func()) ui.Element {
	t := c.Theme()
	b := kit.Press(ui.ButtonBase(c).Label(label).Tooltip(label).
		Size(t.Space(9), t.Space(7)).Center().Radius(t.Radius * 0.7).Cursor(ui.CursorPointer))
	bg, fg := ui.Transparent, t.TextMuted
	if b.Pressed() {
		bg, fg = t.SurfacePressed, t.Text
	} else if b.Hovered() {
		bg, fg = t.SurfaceHover, t.Text
	}
	if danger && b.Hovered() {
		bg, fg = ui.RGB(196, 43, 43), ui.RGB(255, 255, 255)
	}
	b.Background(bg).TextColor(fg).Children(func() {
		ui.Icon(c, lucide(glyph)).Size(14, 14)
	})
	if b.Clicked() {
		if danger {
			sound.Play(sound.Close)
		} else {
			sound.Play(sound.Tap)
		}
		fn()
	}
	return b
}
