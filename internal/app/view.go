// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"fmt"
	"strings"

	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	"github.com/ELY-Open-Source/CrychicUI/navigation"
	"github.com/ELY-Open-Source/CrychicUI/sound"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// View draws the window. It is the only frame callback: the background
// loop asks the window to call it again.
func (a *App) View(c *ui.Context) {
	a.setInvalidate(func() { c.Invalidate() })
	a.syncAura(a.Snapshot())

	s := a.Snapshot()
	if msg := a.takeToast(c.Locale()); msg != "" {
		c.Toast(msg)
	}
	feedback.Toaster(c, feedback.ToasterOptions{Close: i18n.T(c, "common.cancel")})

	ui.Column(c).Fill().Children(func() {
		a.header(c, s)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, s)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				if s.Err != "" && s.Backend == "asusd" {
					alert(c, feedback.ToneDanger, i18n.T(c, "app.backend_offline"), s.Err)
				}
				a.renderPage(c, s)
			})
		})
	})
}

// sidebar draws the navigation.
func (a *App) sidebar(c *ui.Context, s backend.Snapshot) {
	items := []navigation.NavItem{
		{Label: i18n.T(c, "nav.overview"), Icon: "layout-dashboard"},
		{Label: i18n.T(c, "nav.performance"), Icon: "gauge"},
		{Label: i18n.T(c, "nav.fans"), Icon: "fan"},
		{Label: i18n.T(c, "nav.aura"), Icon: "lightbulb"},
		{Label: i18n.T(c, "nav.gpu"), Icon: "monitor"},
		{Label: i18n.T(c, "nav.system"), Icon: "cpu"},
		{Label: i18n.T(c, "nav.about"), Icon: "info"},
	}
	index := pageIndex(a.page)
	rail := navigation.SideNav(c, i18n.T(c, "app.title"), items, &index)
	rail.Shrink(0)
	if index != pageIndex(a.page) {
		a.page = pageName(index)
	}
}

// header draws the title bar above the page.
func (a *App) header(c *ui.Context, s backend.Snapshot) {
	title := i18n.T(c, "app.title")
	subtitle := modelName(c.Locale(), s.Info.Product)
	if subtitle == "" {
		subtitle = s.Backend
	}
	bar := ui.Row(c).FillWidth().Gap(c.Theme().Space(3)).Padding(c.Theme().Space(2), c.Theme().Space(4)).
		AlignItems(ui.Center).Background(c.Theme().Background)
	bar.Children(func() {
		ui.Column(c).Grow(1).Basis(0).Gap(c.Theme().Space(0.5)).Children(func() {
			ui.Text(c, title).Bold().SingleLine()
			if subtitle != "" {
				caption(c, subtitle).SingleLine()
			}
		})
		on := sound.Enabled()
		label, glyph := i18n.T(c, "app.mute"), "volume-2"
		if !on {
			label, glyph = i18n.T(c, "app.unmute"), "volume-x"
		}
		if button.IconButton(c, label, glyph).Clicked() {
			a.setSoundMuted(on)
		}
		if button.IconButton(c, i18n.T(c, "app.refresh"), "refresh-cw").Clicked() {
			go a.refresh()
		}
		ui.Box(c).Width(1).Height(c.Theme().Space(5)).Background(c.Theme().Border)
		a.windowButtons(c)
	})
	bar.DragWindow()
	divider(c)
}

// renderPage draws the page the sidebar chose.
func (a *App) renderPage(c *ui.Context, s backend.Snapshot) {
	switch a.page {
	case "performance":
		a.performancePage(c, s)
	case "fans":
		a.fansPage(c, s)
	case "aura":
		a.auraPage(c, s)
	case "gpu":
		a.gpuPage(c, s)
	case "system":
		a.systemPage(c, s)
	case "about":
		a.aboutPage(c, s)
	default:
		a.overviewPage(c, s)
	}
}

// content wraps a page's body in the scroll view and column every page
// shares, so pages line up.
func content(c *ui.Context, fn func()) ui.Element {
	t := c.Theme()
	return ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(t.Rem(64)).Margin(0, ui.Auto).
			Padding(t.Space(6)).Gap(t.Space(5)).Children(fn)
	})
}

// pageTitle draws a page's title and description.
func pageTitle(c *ui.Context, title, subtitle string) {
	pageHeader(c, title, subtitle, nil)
}

// pageNames lists the pages in the sidebar's order.
var pageNames = []string{"overview", "performance", "fans", "aura", "gpu", "system", "about"}

// pageIndex returns a page's place in the sidebar.
func pageIndex(page string) int {
	for i, name := range pageNames {
		if name == page {
			return i
		}
	}
	return 0
}

// pageName returns the page at an index.
func pageName(i int) string {
	if i < 0 || i >= len(pageNames) {
		return "overview"
	}
	return pageNames[i]
}

// percent formats a percentage.
func percent(v float64) string { return fmt.Sprintf("%.0f%%", v) }

// celsius formats a temperature.
func celsius(v float64) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f °C", v)
}

// rpm formats a fan speed.
func rpm(v int) string {
	if v <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d RPM", v)
}

// profileLabel says a profile in the window's language, falling back to
// its English name.
func profileLabel(c *ui.Context, p backend.Profile) string {
	key := "profile." + string(p)
	if v := i18n.T(c, key); v != key {
		return v
	}
	return p.Label()
}

// ppdLabel says a power-profiles-daemon preset in the window's language.
func ppdLabel(c *ui.Context, p backend.PPDPreset) string {
	if p == "" {
		return i18n.T(c, "common.unknown")
	}
	key := "ppd." + string(p)
	if v := i18n.T(c, key); v != key {
		return v
	}
	return p.Label()
}

// gpuLabel says a graphics mode.
func gpuLabel(c *ui.Context, m backend.GPUMode) string {
	key := "gpu." + string(m)
	if v := i18n.T(c, key); v != key {
		return v
	}
	return m.Label()
}

// profileIndex returns p's place in choices.
func profileIndex(choices []backend.Profile, p backend.Profile) int {
	for i, q := range choices {
		if q == p {
			return i
		}
	}
	return 0
}

// gpuIndex returns m's place in modes.
func gpuIndex(modes []backend.GPUMode, m backend.GPUMode) int {
	for i, q := range modes {
		if q == m {
			return i
		}
	}
	return 0
}

// hasFeature reports whether asusd exposes a firmware attribute.
func hasFeature(s backend.Snapshot, name string) bool {
	f, ok := s.Features[name]
	return ok && f.Supported
}

// featureValue returns a feature's current value.
func featureValue(s backend.Snapshot, name string) int {
	if f, ok := s.Features[name]; ok {
		return f.Value
	}
	return 0
}

// titleCase capitalises a feature name for display when it has no
// translation: ppt_pl1_spl becomes "PPT PL1 SPL".
func titleCase(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if p == "ppt" || p == "nv" || p == "gpu" || p == "dgpu" || p == "pl1" || p == "pl2" {
			parts[i] = strings.ToUpper(p)
		} else {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
