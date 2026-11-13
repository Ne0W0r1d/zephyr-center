// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/sound"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// QuickView returns the tray panel's view. The panel is a separate
// frameless window, so the callbacks close it, open the main window or
// quit the app when the user asks.
func (a *App) QuickView(onOpen, onClose, onQuit func()) func(*ui.Context) {
	return func(c *ui.Context) {
		a.quickView(c, onOpen, onClose, onQuit)
	}
}

// quickView draws the panel: live readings first, then the two switches
// that are useful without opening the full window.
func (a *App) quickView(c *ui.Context, onOpen, onClose, onQuit func()) {
	s := a.Snapshot()
	t := c.Theme()

	var esc ui.Element
	root := ui.Column(c).Fill().Background(t.Background).
		Padding(t.Space(4)).Gap(t.Space(3.5))
	root.Children(func() {
		a.quickHeader(c, s, onClose)
		a.quickMetrics(c, s)
		a.quickGPUState(c, s)
		a.quickProfiles(c, s)
		a.quickGPUChoices(c, s)
		ui.Box(c).Grow(1)
		a.quickFooter(c, onOpen, onQuit)
		esc = ui.Box(c).Size(0, 0)
	})
	if esc.OverlayShortcut(0, ui.KeyEscape) && onClose != nil {
		onClose()
	}
}

// quickHeader names the app and machine, with the panel's own close
// control so it can be dismissed without reaching for the tray again.
func (a *App) quickHeader(c *ui.Context, s backend.Snapshot, onClose func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2.5)).Children(func() {
		logo := ui.Box(c).Size(t.Space(8), t.Space(8)).Radius(t.Radius * 0.8).
			Background(t.Accent).Center()
		logo.Children(func() {
			ui.Icon(c, lucide("zap")).Size(18, 18).TextColor(t.AccentText)
		})

		ui.Column(c).Grow(1).Basis(0).Gap(t.Space(0.5)).Children(func() {
			ui.Text(c, i18n.T(c, "app.title")).Bold().SingleLine()
			name := modelName(c.Locale(), s.Info.Product)
			if name == "" {
				name = s.Backend
			}
			caption(c, name).SingleLine()
		})
		if button.IconButton(c, i18n.T(c, "app.close"), "x").Clicked() {
			if onClose != nil {
				onClose()
			}
		}
	})
}

// quickMetrics shows the CPU and GPU temperatures and fan speeds as four
// compact cards.
func (a *App) quickMetrics(c *ui.Context, s backend.Snapshot) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
		quickMetric(c, "cpu", i18n.T(c, "overview.cpu"), celsius(s.Sensors.CPUTemp))
		quickMetric(c, "thermometer", i18n.T(c, "overview.gpu"), celsius(s.Sensors.GPUTemp))
	})
	ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
		quickMetric(c, "fan", i18n.T(c, "overview.fan_cpu"), rpm(s.Sensors.CPURPM))
		quickMetric(c, "fan", i18n.T(c, "overview.fan_gpu"), rpm(s.Sensors.GPURPM))
	})
}

// quickMetric draws one reading. The cards share the row evenly.
func quickMetric(c *ui.Context, glyph, label, value string) {
	t := c.Theme()
	card := kit.Card(c).Grow(1).Basis(0).Padding(t.Space(3)).Gap(t.Space(1))
	card.Children(func() {
		ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide(glyph)).Size(14, 14).TextColor(t.TextMuted)
			caption(c, label).SingleLine()
		})
		kit.Numeric(c, value).Bold().FontSize(t.Rem(1.25)).SingleLine()
	})
}

// quickGPUState names the graphics mode in use, and flags a mode that is
// queued for the next boot.
func (a *App) quickGPUState(c *ui.Context, s backend.Snapshot) {
	t := c.Theme()
	card := kit.Card(c).FillWidth().Padding(t.Space(3)).Gap(t.Space(1))
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
			ui.Icon(c, lucide("monitor")).Size(14, 14).TextColor(t.TextMuted)
			caption(c, i18n.T(c, "gpu.in_use")).Grow(1)
			if s.GPU.Supported && s.GPU.NeedsReboot() {
				badge(c, i18n.T(c, "quick.reboot"), badgeWarning)
			}
		})
		mode := gpuLabel(c, s.GPU.Mode)
		if !s.GPU.Supported {
			mode = i18n.T(c, "common.unavailable")
		}
		ui.Text(c, mode).FontWeight(600).SingleLine()
	})
}

// quickProfiles offers every firmware profile in one row. The usual three
// modes line up without a second row pushing the footer against the edge.
func (a *App) quickProfiles(c *ui.Context, s backend.Snapshot) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Text(c, i18n.T(c, "quick.performance")).FontWeight(600)
		ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
			for _, p := range s.ProfileChoices {
				mode := p
				quickChoice(c, profileLabel(c, mode), mode == s.Profile, func() {
					a.setProfile(mode)
				}).Grow(1).Basis(0).MinWidth(0)
			}
		})
	})
}

// quickGPUChoices switches the graphics mode, also on one line.
func (a *App) quickGPUChoices(c *ui.Context, s backend.Snapshot) {
	t := c.Theme()
	if !s.GPU.Supported {
		return
	}
	modes := s.GPU.Modes
	if len(modes) == 0 {
		modes = []backend.GPUMode{backend.GPUModeIntegrated, backend.GPUModeHybrid, backend.GPUModeUltimate}
	}
	ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
		ui.Text(c, i18n.T(c, "quick.graphics")).FontWeight(600)
		ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
			for _, m := range modes {
				mode := m
				quickChoice(c, gpuLabel(c, mode), mode == s.GPU.Mode, func() {
					a.setGPU(mode)
				}).Grow(1).Basis(0).MinWidth(0)
			}
		})
	})
}

// quickFooter keeps opening the full window at the left and quitting at
// the bottom right.
func (a *App) quickFooter(c *ui.Context, onOpen, onQuit func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		if button.Ghost(c, i18n.T(c, "quick.open")).Clicked() && onOpen != nil {
			onOpen()
		}
		ui.Box(c).Grow(1)
		if button.Destructive(c, i18n.T(c, "quick.quit")).Clicked() && onQuit != nil {
			onQuit()
		}
	})
}

// quickChoice is a full-width toggle with the accent as its selected
// state.
func quickChoice(c *ui.Context, label string, selected bool, onSelect func()) ui.Element {
	t := c.Theme()
	b := kit.Press(ui.ButtonBase(c).Label(label).Tooltip(label).
		Padding(t.Space(2), t.Space(2)).Radius(t.Radius * 0.7).
		Cursor(ui.CursorPointer).Center())
	bg, fg, border := t.Surface, t.Text, t.Border
	if selected {
		bg, fg, border = t.Accent, t.AccentText, t.Accent
	} else if b.Hovered() {
		bg = t.SurfaceHover
	}
	b.Background(bg).Border(1, border).TextColor(fg).Children(func() {
		ui.Text(c, label).SingleLine()
	})
	if b.Clicked() && !selected {
		sound.Play(sound.Select)
		onSelect()
		c.Invalidate()
	}
	return b
}
