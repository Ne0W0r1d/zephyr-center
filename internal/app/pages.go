// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/choice"
	"github.com/ELY-Open-Source/CrychicUI/color"
	"github.com/ELY-Open-Source/CrychicUI/display"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/ELY-Open-Source/CrychicUI/typography"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// overviewPage shows the machine at a glance.
func (a *App) overviewPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "overview.title"), i18n.TF(c, "overview.subtitle", map[string]any{
			"product": orUnknown(modelName(c.Locale(), s.Info.Product), i18n.T(c, "common.unknown")),
			"backend": s.Backend,
		}))
		a.profileCard(c, s)
		ui.Row(c).FillWidth().Gap(c.Theme().Space(3)).Wrap().AlignItems(ui.Stretch).Children(func() {
			metricCard(c, i18n.T(c, "overview.cpu"), celsius(s.Sensors.CPUTemp), i18n.T(c, "overview.load")+" "+percent(s.Sensors.CPUUsage), "thermometer")
			metricCard(c, i18n.T(c, "overview.gpu"), celsius(s.Sensors.GPUTemp), gpuLabel(c, s.GPU.Mode), "thermometer")
			metricCard(c, i18n.T(c, "overview.fan_cpu"), rpm(s.Sensors.CPURPM), i18n.T(c, "overview.fans"), "fan")
			metricCard(c, i18n.T(c, "overview.fan_gpu"), rpm(s.Sensors.GPURPM), i18n.T(c, "overview.fans"), "fan")
		})
		ui.Row(c).FillWidth().Gap(c.Theme().Space(3)).Wrap().AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Basis(320).Children(func() { a.batteryCard(c, s) })
			ui.Column(c).Grow(1).Basis(320).Children(func() { a.quickCard(c, s) })
		})
	})
}

// profileCard is the overview's large profile chooser.
func (a *App) profileCard(c *ui.Context, s backend.Snapshot) {
	options := s.ProfileChoices
	if len(options) == 0 {
		options = []backend.Profile{backend.ProfileBalanced}
	}
	idx := profileIndex(options, s.Profile)
	labels := make([]string, len(options))
	for i, p := range options {
		labels[i] = profileLabel(c, p)
	}
	settingsCard(c, func() {
		settingRow(c, i18n.T(c, "overview.profile"), i18n.T(c, "overview.profile_desc"), func() {
			if segments(c, i18n.T(c, "overview.profile"), labels, &idx) &&
				idx >= 0 && idx < len(options) && options[idx] != s.Profile {
				a.setProfile(options[idx])
			}
		})
	})
}

// metricCard draws one measurement.
func metricCard(c *ui.Context, label, value, sub, glyph string) {
	ui.Column(c).Width(198).Children(func() {
		settingsCard(c, func() {
			ui.Row(c).Gap(c.Theme().Space(1.5)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide(glyph)).Size(16, 16).TextColor(c.Theme().TextMuted)
				caption(c, label)
			})
			kit.Numeric(c, value).Bold().FontSize(c.Theme().Rem(1.4))
			caption(c, sub).SingleLine()
		})
	})
}

// batteryCard shows charge, health and draw.
func (a *App) batteryCard(c *ui.Context, s backend.Snapshot) {
	b := s.Battery
	settingsCard(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, i18n.T(c, "overview.battery")).Bold().Grow(1)
			if b.Present {
				badge(c, batteryState(c, b), badgeNeutral)
			}
		})
		if !b.Present {
			muted(c, i18n.T(c, "common.unavailable"))
			return
		}
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				kit.Numeric(c, percent(float64(b.Capacity))).Bold().FontSize(c.Theme().Rem(1.6)).Grow(1)
				muted(c, batteryStatus(c, b))
			})
			feedback.ProgressBar(c, i18n.T(c, "overview.battery"), float64(b.Capacity)/100)
		})
		divider(c)
		ui.Row(c).Gap(24).Children(func() {
			stat(c, i18n.T(c, "overview.battery_health"), percent(b.Health))
			stat(c, i18n.T(c, "overview.battery_cycles"), strconv.Itoa(b.Cycles))
			if b.TimeLeft != "" {
				stat(c, i18n.T(c, "overview.battery_time"), b.TimeLeft)
			}
		})
	})
}

// quickCard holds the switches for the firmware attributes that have one.
func (a *App) quickCard(c *ui.Context, s backend.Snapshot) {
	settingsCard(c, func() {
		ui.Text(c, i18n.T(c, "overview.quick")).Bold()
		if hasFeature(s, "panel_overdrive") {
			on := featureValue(s, "panel_overdrive") == 1
			if switchRow(c, i18n.T(c, "overview.panel_overdrive"), i18n.T(c, "overview.panel_overdrive_desc"), &on) {
				a.setFeature("panel_overdrive", boolInt(on))
			}
		}
		if hasFeature(s, "boot_sound") {
			on := featureValue(s, "boot_sound") == 1
			if switchRow(c, i18n.T(c, "overview.boot_sound"), i18n.T(c, "overview.boot_sound_desc"), &on) {
				a.setFeature("boot_sound", boolInt(on))
			}
		}
		if !hasFeature(s, "panel_overdrive") && !hasFeature(s, "boot_sound") {
			muted(c, i18n.T(c, "common.not_supported"))
		}
	})
}

// performancePage sets the profile, the charge limit and the power limits.
func (a *App) performancePage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "performance.title"), i18n.T(c, "performance.subtitle"))
		section(c, i18n.T(c, "performance.profiles"), i18n.T(c, "performance.profiles_desc"), func() {
			sel := string(s.Profile)
			opts := make([]struct{ Value, Title, Body string }, 0, len(s.ProfileChoices))
			for _, p := range s.ProfileChoices {
				opts = append(opts, struct{ Value, Title, Body string }{string(p), profileLabel(c, p), profileDescription(c, p)})
			}
			if optionCards(c, opts, &sel) && sel != string(s.Profile) {
				a.setProfile(backend.Profile(sel))
			}
		})
		a.chargeCard(c, s)
		a.powerCard(c, s)
	})
}

// chargeCard caps the battery's charge.
func (a *App) chargeCard(c *ui.Context, s backend.Snapshot) {
	if !s.ChargeSupported {
		return
	}
	if !a.chargeSet {
		a.chargeDraft = float64(s.ChargeLimit)
		a.chargeSet = true
	}
	if v := float64(s.ChargeLimit); v != a.chargeDraft && a.chargeDraft == 0 {
		a.chargeDraft = v
	}
	section(c, i18n.T(c, "performance.charge"), i18n.T(c, "performance.charge_desc"), func() {
		settingsCard(c, func() {
			settingRow(c, i18n.T(c, "performance.charge_limit"), "", func() {
				ui.Column(c).Width(c.Theme().Rem(22)).Children(func() {
					sl := choice.ValueSlider(c, i18n.T(c, "performance.charge_limit"), &a.chargeDraft, 20, 100,
						choice.ValueSliderOptions{Step: 1, Format: "%.0f%%"})
					if sl.Changed() {
						limit := int(a.chargeDraft + 0.5)
						if limit != s.ChargeLimit {
							a.setCharge(limit)
						}
					}
				})
			})
		})
	})
}

// powerCard shows the power limits asusd exposes.
func (a *App) powerCard(c *ui.Context, s backend.Snapshot) {
	rows := []struct{ key, title string }{
		{"ppt_pl1_spl", "performance.ppt_pl1"},
		{"ppt_pl2_sppt", "performance.ppt_pl2"},
		{"nv_dynamic_boost", "performance.nv_boost"},
		{"nv_temp_target", "performance.nv_temp"},
	}
	var available []struct{ key, title string }
	for _, r := range rows {
		if hasFeature(s, r.key) {
			available = append(available, r)
		}
	}
	if len(available) == 0 {
		return
	}
	section(c, i18n.T(c, "performance.power"), i18n.T(c, "performance.power_desc"), func() {
		rows := make([]func(), 0, len(available))
		for _, r := range available {
			r := r
			rows = append(rows, func() {
				f := s.Features[r.key]
				lo, hi := float64(f.Min), float64(f.Max)
				step := float64(f.Step)
				if step <= 0 {
					step = 1
				}
				if lo < 0 {
					lo = 0
				}
				if hi <= lo {
					hi = lo + 100
				}
				v := float64(f.Value)
				settingRow(c, i18n.T(c, r.title), "", func() {
					if choice.NumberStepper(c, i18n.T(c, r.title), &v, lo, hi, step,
						choice.NumberStepperOptions{}).Changed() {
						next := int(v + 0.5)
						if next != f.Value {
							a.setFeature(r.key, next)
						}
					}
				})
			})
		}
		grouped(c, rows)
	})
}

// fansPage edits each profile's fan curves.
func (a *App) fansPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "fans.title"), i18n.T(c, "fans.subtitle"))
		profiles := s.ProfileChoices
		if len(profiles) == 0 {
			profiles = []backend.Profile{backend.ProfileBalanced}
		}
		pIdx := profileIndex(profiles, backend.Profile(a.fanPage))
		labels := make([]string, len(profiles))
		for i, p := range profiles {
			labels[i] = profileLabel(c, p)
		}
		if segments(c, i18n.T(c, "fans.profile"), labels, &pIdx) && pIdx >= 0 && pIdx < len(profiles) {
			a.fanPage = string(profiles[pIdx])
			a.fanFan = firstFan(s, profiles[pIdx])
		}
		profile := backend.Profile(a.fanPage)
		curves := s.Curves[profile]
		fans := []string{}
		seen := map[string]bool{}
		for _, cv := range curves {
			if !seen[cv.Fan] {
				seen[cv.Fan] = true
				fans = append(fans, cv.Fan)
			}
		}
		if len(fans) == 0 {
			emptyState(c, "fan", i18n.T(c, "fans.unavailable"), "")
			return
		}
		if !seen[a.fanFan] {
			a.fanFan = fans[0]
		}
		fIdx := 0
		for i, f := range fans {
			if f == a.fanFan {
				fIdx = i
			}
		}
		flabels := make([]string, len(fans))
		for i, f := range fans {
			flabels[i] = fanLabel(c, f)
		}
		if segments(c, i18n.T(c, "fans.fan"), flabels, &fIdx) && fIdx >= 0 && fIdx < len(fans) {
			a.fanFan = fans[fIdx]
		}
		var curve backend.FanCurve
		for _, cv := range curves {
			if cv.Fan == a.fanFan {
				curve = cv
			}
		}
		key := string(profile) + "/" + a.fanFan
		pts, ok := a.fanDraft[key]
		if !ok {
			pts = curve.Points()
			a.fanDraft[key] = pts
		}
		enabled, ok := a.fanEnable[key]
		if !ok {
			enabled = curve.Enabled
			a.fanEnable[key] = enabled
		}
		settingsCard(c, func() {
			ui.Text(c, fanLabel(c, a.fanFan)).Bold()
			caption(c, i18n.T(c, "fans.help")).MaxWidth(620)
			a.curveEditor(c, key, &pts)
			divider(c)
			if switchRow(c, i18n.T(c, "fans.enabled"), i18n.T(c, "fans.enabled_desc"), &enabled) {
				a.fanEnable[key] = enabled
				a.do(i18n.T(c, "common.applied"), func(ctx context.Context) error {
					return a.be.SetFanCurvesEnabled(ctx, profile, enabled)
				})
			}
			ui.Row(c).FillWidth().Gap(c.Theme().Space(2)).Justify(ui.End).Children(func() {
				if button.Outline(c, i18n.T(c, "fans.reset")).Clicked() {
					a.do(i18n.T(c, "fans.reset_done"), func(ctx context.Context) error {
						return a.be.ResetFanCurves(ctx, profile)
					})
				}
				if button.Solid(c, i18n.T(c, "fans.apply")).Clicked() {
					next := curve
					next.SetPoints(pts)
					next.Enabled = enabled
					a.fanDraft[key] = pts
					a.do(i18n.T(c, "fans.applied"), func(ctx context.Context) error {
						return a.be.SetFanCurve(ctx, profile, next)
					})
				}
			})
		})
	})
}

// auraPage edits the keyboard lighting.
func (a *App) auraPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageHeader(c, i18n.T(c, "aura.title"), i18n.T(c, "aura.subtitle"), func() {
			if s.Aura.Supported && button.Solid(c, i18n.T(c, "common.apply")).Clicked() {
				a.applyAura()
			}
		})
		if !s.Aura.Supported {
			emptyState(c, "lightbulb", i18n.T(c, "aura.unavailable"), "")
			return
		}
		// Brightness.
		section(c, i18n.T(c, "aura.brightness"), "", func() {
			values := s.Aura.Brightnesses
			if len(values) == 0 {
				values = []uint32{0, 1, 2, 3}
			}
			labels := make([]string, len(values))
			for i, v := range values {
				labels[i] = brightnessLabel(c, v)
			}
			idx := 0
			for i, v := range values {
				if v == s.Aura.Brightness {
					idx = i
				}
			}
			settingsCard(c, func() {
				settingRow(c, i18n.T(c, "aura.level"), "", func() {
					if !segments(c, i18n.T(c, "aura.level"), labels, &idx) ||
						idx < 0 || idx >= len(values) || values[idx] == s.Aura.Brightness {
						return
					}
					a.do(i18n.T(c, "aura.applied"), func(ctx context.Context) error {
						return a.be.SetAuraBrightness(ctx, values[idx])
					})
				})
			})
		})
		layout.Columns(c, []func(){
			func() { a.auraModeSection(c, s) },
			func() {
				a.auraColorSection(c)
				a.auraPowerSection(c, s)
			},
		})
	})
}

// auraModeSection holds the effect, zone, speed and direction.
func (a *App) auraModeSection(c *ui.Context, s backend.Snapshot) {
	section(c, i18n.T(c, "aura.mode"), "", func() {
		settingsCard(c, func() {
			options := s.Aura.Modes
			if len(options) == 0 {
				options = []uint32{0}
			}
			names := make([]string, len(options))
			for i, m := range options {
				names[i] = auraModeLabel(c, m)
			}
			sel := auraModeLabel(c, a.auraMode)
			if dropdown(c, i18n.T(c, "aura.mode"), names, &sel) {
				for i, n := range names {
					if n == sel {
						a.auraMode = options[i]
						a.auraDirty = true
					}
				}
			}
			zoneOptions := s.Aura.Zones
			if len(zoneOptions) == 0 {
				zoneOptions = []uint32{0}
			}
			zoneNames := make([]string, len(zoneOptions))
			for i, z := range zoneOptions {
				zoneNames[i] = auraZoneLabel(c, z)
			}
			zsel := auraZoneLabel(c, a.auraZone)
			if dropdown(c, i18n.T(c, "aura.zone"), zoneNames, &zsel) {
				for i, n := range zoneNames {
					if n == zsel {
						a.auraZone = zoneOptions[i]
						a.auraDirty = true
					}
				}
			}
			speeds := []string{i18n.T(c, "aura.speed.slow"), i18n.T(c, "aura.speed.normal"), i18n.T(c, "aura.speed.fast")}
			speed := speedLabel(c, a.auraSpeed)
			if dropdown(c, i18n.T(c, "aura.speed"), speeds, &speed) {
				a.auraSpeed = speedCode(speed)
				a.auraDirty = true
			}
			dirs := []string{i18n.T(c, "aura.direction.right"), i18n.T(c, "aura.direction.left"), i18n.T(c, "aura.direction.up"), i18n.T(c, "aura.direction.down")}
			dir := directionLabel(c, a.auraDirection)
			if dropdown(c, i18n.T(c, "aura.direction"), dirs, &dir) {
				a.auraDirection = dirCode(dir)
				a.auraDirty = true
			}
		})
	})
}

// auraColorSection holds the two colour wells.
func (a *App) auraColorSection(c *ui.Context) {
	section(c, i18n.T(c, "aura.colours"), "", func() {
		settingsCard(c, func() {
			grouped(c, []func(){
				func() {
					settingRow(c, i18n.T(c, "aura.color1"), "", func() {
						if color.Well(c, i18n.T(c, "aura.color1"), &a.auraColor1).Changed() {
							a.auraDirty = true
						}
					})
				},
				func() {
					settingRow(c, i18n.T(c, "aura.color2"), "", func() {
						if color.Well(c, i18n.T(c, "aura.color2"), &a.auraColor2).Changed() {
							a.auraDirty = true
						}
					})
				},
			})
		})
	})
}

// auraPowerSection holds the power states per zone.
func (a *App) auraPowerSection(c *ui.Context, s backend.Snapshot) {
	section(c, i18n.T(c, "aura.power"), i18n.T(c, "aura.power_desc"), func() {
		settingsCard(c, func() {
			for _, z := range s.Aura.PowerZones {
				p := a.auraPower[z]
				p.Zone = z
				changed := false
				ui.Text(c, auraPowerZoneLabel(c, z)).Bold()
				ui.Grid(c).FillWidth().Columns(2).Gap(c.Theme().Space(2)).Children(func() {
					changed = a.powerSwitch(c, i18n.T(c, "aura.power_boot"), &p.Boot) || changed
					changed = a.powerSwitch(c, i18n.T(c, "aura.power_awake"), &p.Awake) || changed
					changed = a.powerSwitch(c, i18n.T(c, "aura.power_sleep"), &p.Sleep) || changed
					changed = a.powerSwitch(c, i18n.T(c, "aura.power_shutdown"), &p.Shutdown) || changed
				})
				if changed {
					a.auraPower[z] = p
					pp := p
					a.do(i18n.T(c, "aura.applied"), func(ctx context.Context) error {
						return a.be.SetAuraPower(ctx, pp)
					})
				}
			}
		})
	})
}

// auraPowerZoneLabel names the zone a power row turns on and off.
func auraPowerZoneLabel(c *ui.Context, zone uint32) string {
	switch zone {
	case 0:
		return i18n.T(c, "aura.zone.none")
	case 1:
		return i18n.T(c, "aura.zone.none")
	case 2:
		return i18n.T(c, "aura.zone.bar_left") + " / " + i18n.T(c, "aura.zone.bar_right")
	default:
		return auraZoneLabel(c, zone)
	}
}

// powerSwitch is a compact labelled switch.
func (a *App) powerSwitch(c *ui.Context, label string, on *bool) bool {
	return choice.Toggle(c, label, on).Changed()
}

// applyAura writes the effect the user shaped.
func (a *App) applyAura() {
	m := backend.AuraMode{
		Mode:      a.auraMode,
		Zone:      a.auraZone,
		Color1:    [3]byte{a.auraColor1.R, a.auraColor1.G, a.auraColor1.B},
		Color2:    [3]byte{a.auraColor2.R, a.auraColor2.G, a.auraColor2.B},
		Speed:     a.auraSpeed,
		Direction: a.auraDirection,
	}
	a.auraDirty = false
	a.do("aura.applied", func(ctx context.Context) error {
		return a.be.SetAura(ctx, m)
	})
}

// gpuPage switches graphics modes.
func (a *App) gpuPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "gpu.title"), i18n.T(c, "gpu.subtitle"))
		if !s.GPU.Supported {
			emptyState(c, "monitor", i18n.T(c, "gpu.unavailable"), "")
			return
		}
		if s.GPU.NeedsReboot() {
			alert(c, feedback.ToneWarning,
				i18n.TF(c, "gpu.queued", map[string]any{"mode": gpuLabel(c, s.GPU.Pending)}),
				i18n.T(c, "gpu.reboot"))
		}
		section(c, i18n.T(c, "gpu.mode"), "", func() {
			sel := string(s.GPU.Mode)
			opts := make([]struct{ Value, Title, Body string }, 0, len(s.GPU.Modes))
			for _, m := range s.GPU.Modes {
				opts = append(opts, struct{ Value, Title, Body string }{string(m), gpuLabel(c, m), gpuDescription(c, m)})
			}
			if optionCards(c, opts, &sel) && sel != string(s.GPU.Mode) {
				a.setGPU(backend.GPUMode(sel))
			}
		})
		settingsCard(c, func() {
			settingRow(c, i18n.T(c, "gpu.in_use"), gpuLabel(c, s.GPU.Mode), nil)
			if s.GPU.NeedsReboot() {
				divider(c)
				settingRow(c, i18n.T(c, "gpu.after_restart"), gpuLabel(c, s.GPU.Pending), nil)
			}
		})
		caption(c, i18n.T(c, "gpu.reboot"))
	})
}

// systemPage lists the hardware and the firmware attributes.
func (a *App) systemPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "system.title"), i18n.T(c, "system.subtitle"))
		pairs := []display.KeyValuePair{
			{Key: i18n.T(c, "system.model"), Value: s.Info.Product},
			{Key: i18n.T(c, "system.board"), Value: s.Info.Board},
			{Key: i18n.T(c, "system.cpu"), Value: s.Info.CPU},
			{Key: i18n.T(c, "system.igpu"), Value: s.Info.IGPU},
			{Key: i18n.T(c, "system.dgpu"), Value: s.Info.DGPU},
			{Key: i18n.T(c, "system.bios"), Value: s.Info.BIOS},
			{Key: i18n.T(c, "system.kernel"), Value: s.Info.Kernel},
			{Key: i18n.T(c, "system.os"), Value: s.Info.OS},
			{Key: i18n.T(c, "system.backend"), Value: s.Backend},
			{Key: i18n.T(c, "system.daemon"), Value: s.DaemonVersion},
		}
		display.DescriptionList(c, nonEmpty(pairs))
		if len(s.Features) > 0 {
			section(c, i18n.T(c, "system.features"), i18n.T(c, "system.features_desc"), func() {
				grouped(c, featureRows(c, s))
			})
		}
	})
}

// featureRows lists the firmware attributes as the firmware reports them,
// each value written the way the setting reads rather than as a raw
// number. The list is read-only: asusd decides which attributes can be
// written, and several of these — charge_mode among them — cannot.
func featureRows(c *ui.Context, s backend.Snapshot) []func() {
	var rows []func()
	for _, name := range featureNames(s) {
		f := s.Features[name]
		value := f.Value
		if f.Queued >= 0 && f.Queued != f.Value {
			// The daemon holds a value for the next boot; show both.
			value = f.Queued
		}
		label := featureValueLabel(c, name, value)
		if f.Queued >= 0 && f.Queued != f.Value {
			label = i18n.TF(c, "system.features_queued", map[string]any{"value": label})
		}
		title, label := featureTitle(c, name), label
		rows = append(rows, func() {
			ui.Row(c).FillWidth().Gap(c.Theme().Space(4)).AlignItems(ui.Start).Children(func() {
				ui.Text(c, title).Grow(1).Basis(0)
				ui.Text(c, label).Shrink(0).TextAlign(ui.End)
			})
		})
	}
	return rows
}

// featureTitle names a firmware attribute for the user, falling back to
// its readable attribute name.
func featureTitle(c *ui.Context, name string) string {
	key := "feature." + name
	if v := i18n.T(c, key); v != key {
		return v
	}
	return titleCase(name)
}

// featureDescription explains a firmware attribute.
func featureDescription(c *ui.Context, name string) string {
	key := "feature." + name + "_desc"
	if v := i18n.T(c, key); v != key {
		return v
	}
	return ""
}

// featureUnit returns a number attribute's format and unit.
func featureUnit(name string) (format, unit string) {
	switch name {
	case "ppt_pl1_spl", "ppt_pl2_sppt", "nv_dynamic_boost", "dgpu_tgp":
		return "%.0f W", "W"
	case "nv_temp_target":
		return "%.0f °C", "°C"
	default:
		return "%.0f", ""
	}
}

// featureValueLabel says a firmware value the way its row shows it.
func featureValueLabel(c *ui.Context, name string, value int) string {
	switch name {
	case "charge_mode":
		return chargeModeLabel(c, value)
	case "boot_sound", "panel_overdrive", "dgpu_disable":
		if value == 1 {
			return i18n.T(c, "common.on")
		}
		return i18n.T(c, "common.off")
	case "gpu_mux_mode":
		// asusd's mux is 0 for the discrete GPU and 1 for the hybrid.
		if value == 0 {
			return i18n.T(c, "gpu.ultimate")
		}
		return i18n.T(c, "gpu.hybrid")
	}
	format, _ := featureUnit(name)
	if format == "%.0f W" {
		return i18n.TF(c, "feature.watts", map[string]any{"value": value})
	}
	if format == "%.0f °C" {
		return i18n.TF(c, "feature.degrees", map[string]any{"value": value})
	}
	return strconv.Itoa(value)
}

// chargeModeLabel names a charging mode: 0 battery care, 1 the adapter,
// 2 USB PD, 3 the adapter first, 6 low-power USB or unknown.
func chargeModeLabel(c *ui.Context, value int) string {
	key := "feature.charge." + strconv.Itoa(value)
	if v := i18n.T(c, key); v != key {
		return v
	}
	return i18n.TF(c, "feature.charge.unknown", map[string]any{"value": value})
}

// nonEmpty drops the rows whose value is empty.
func nonEmpty(pairs []display.KeyValuePair) []display.KeyValuePair {
	out := pairs[:0]
	for _, p := range pairs {
		if p.Value != "" {
			out = append(out, p)
		}
	}
	return out
}

// aboutPage shows versions, the language picker and the licences.
func (a *App) aboutPage(c *ui.Context, s backend.Snapshot) {
	content(c, func() {
		pageTitle(c, i18n.T(c, "about.title"), i18n.T(c, "about.subtitle"))
		display.DescriptionList(c, nonEmpty([]display.KeyValuePair{
			{Key: i18n.T(c, "about.version"), Value: mygo.App.Version()},
			{Key: i18n.T(c, "system.backend"), Value: s.Backend},
			{Key: i18n.T(c, "system.daemon"), Value: s.DaemonVersion},
		}))
		section(c, i18n.T(c, "about.language"), i18n.T(c, "about.language_desc"), func() {
			langs := append([]string{"system"}, i18n.Languages()...)
			names := make([]string, len(langs))
			for i, l := range langs {
				if l == "system" {
					names[i] = i18n.T(c, "about.follow_system")
				} else {
					names[i] = i18n.Name(l)
				}
			}
			sel := names[0]
			for i, l := range langs {
				if l != "system" && l == a.localeChoice {
					sel = names[i]
				}
			}
			settingsCard(c, func() {
				settingRow(c, i18n.T(c, "about.language"), "", func() {
					if !dropdown(c, i18n.T(c, "about.language"), names, &sel) {
						return
					}
					for i, n := range names {
						if n != sel {
							continue
						}
						if langs[i] == "system" {
							a.localeChoice = ""
							mygo.App.SetLocale("")
						} else {
							a.localeChoice = langs[i]
							mygo.App.SetLocale(langs[i])
						}
					}
				})
			})
		})
		section(c, i18n.T(c, "about.license"), i18n.T(c, "about.license_desc"), func() {
			settingsCard(c, func() {
				typography.InlineCode(c, "MPL-2.0 OR GPL-3.0-or-later")
			})
		})
		caption(c, i18n.T(c, "about.thanks"))
	})
}

// syncAura copies the daemon's Aura state into the draft once, and again
// whenever the daemon reports something the draft does not hold.
func (a *App) syncAura(s backend.Snapshot) {
	if !s.Aura.Supported {
		return
	}
	if !a.auraSynced && !a.auraDirty {
		a.auraSynced = true
		a.auraMode = s.Aura.Mode.Mode
		a.auraZone = s.Aura.Mode.Zone
		a.auraSpeed = s.Aura.Mode.Speed
		a.auraDirection = s.Aura.Mode.Direction
		a.auraColor1 = ui.RGB(s.Aura.Mode.Color1[0], s.Aura.Mode.Color1[1], s.Aura.Mode.Color1[2])
		a.auraColor2 = ui.RGB(s.Aura.Mode.Color2[0], s.Aura.Mode.Color2[1], s.Aura.Mode.Color2[2])
	}
	if len(a.auraPower) == 0 {
		for _, p := range s.Aura.Power {
			a.auraPower[p.Zone] = p
		}
	}
}

// fanLabel names a fan.
func fanLabel(c *ui.Context, fan string) string {
	switch fan {
	case "CPU":
		return i18n.T(c, "fans.cpu")
	case "GPU":
		return i18n.T(c, "fans.gpu")
	case "MID":
		return i18n.T(c, "fans.mid")
	default:
		return fan
	}
}

// firstFan picks the first fan of a profile.
func firstFan(s backend.Snapshot, p backend.Profile) string {
	if cs := s.Curves[p]; len(cs) > 0 {
		return cs[0].Fan
	}
	return "CPU"
}

// featureNames sorts the firmware attributes for display.
func featureNames(s backend.Snapshot) []string {
	names := make([]string, 0, len(s.Features))
	for name := range s.Features {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func orUnknown(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// profileDescription says what a profile does.
func profileDescription(c *ui.Context, p backend.Profile) string {
	key := "profile." + string(p) + "_desc"
	if v := i18n.T(c, key); v != key {
		return v
	}
	return ""
}

// gpuDescription says what a graphics mode does.
func gpuDescription(c *ui.Context, m backend.GPUMode) string {
	key := "gpu." + string(m) + "_desc"
	if v := i18n.T(c, key); v != key {
		return v
	}
	return ""
}

// batteryStatus says the battery's state.
func batteryStatus(c *ui.Context, b backend.Battery) string {
	switch b.Status {
	case "Charging":
		return i18n.T(c, "overview.battery_charging")
	case "Discharging":
		return i18n.T(c, "overview.battery_discharging")
	case "Full", "Not charging":
		return i18n.T(c, "overview.battery_full")
	default:
		return b.Status
	}
}

// batteryState is the badge on the battery card.
func batteryState(c *ui.Context, b backend.Battery) string {
	if b.Status == "Discharging" {
		return i18n.T(c, "overview.battery_discharging")
	}
	return i18n.T(c, "overview.battery_charging")
}

// stat is one labelled number.
func stat(c *ui.Context, label, value string) {
	ui.Column(c).Gap(c.Theme().Space(0.5)).Children(func() {
		caption(c, label)
		kit.Numeric(c, value)
	})
}

// auraModeLabel names an Aura mode.
func auraModeLabel(c *ui.Context, mode uint32) string {
	key := map[uint32]string{
		0: "aura.mode.static", 1: "aura.mode.breathe", 2: "aura.mode.rainbow_cycle",
		3: "aura.mode.rainbow_wave", 4: "aura.mode.star", 5: "aura.mode.rain",
		6: "aura.mode.highlight", 7: "aura.mode.laser", 8: "aura.mode.ripple",
		10: "aura.mode.pulse", 11: "aura.mode.comet", 12: "aura.mode.flash",
	}[mode]
	if key == "" {
		return fmt.Sprintf("#%d", mode)
	}
	return i18n.T(c, key)
}

// brightnessLabel names a brightness step.
func brightnessLabel(c *ui.Context, v uint32) string {
	key := map[uint32]string{0: "aura.brightness.off", 1: "aura.brightness.low", 2: "aura.brightness.med", 3: "aura.brightness.high"}[v]
	if key == "" {
		return strconv.Itoa(int(v))
	}
	return i18n.T(c, key)
}

// auraZoneLabel names an Aura zone.
func auraZoneLabel(c *ui.Context, z uint32) string {
	key := map[uint32]string{
		0: "aura.zone.none", 1: "aura.zone.key1", 2: "aura.zone.key2", 3: "aura.zone.key3",
		4: "aura.zone.key4", 5: "aura.zone.logo", 6: "aura.zone.bar_left", 7: "aura.zone.bar_right",
	}[z]
	if key == "" {
		return fmt.Sprintf("#%d", z)
	}
	return i18n.T(c, key)
}

// speedLabel says an Aura speed in the window's language.
func speedLabel(c *ui.Context, speed string) string {
	switch speed {
	case "Slow":
		return i18n.T(c, "aura.speed.slow")
	case "Fast":
		return i18n.T(c, "aura.speed.fast")
	default:
		return i18n.T(c, "aura.speed.normal")
	}
}

// speedCode turns a translated speed back into asusd's name.
func speedCode(label string) string {
	switch label {
	case "Slow", "Langsam", "慢速", "遅い":
		return "Slow"
	case "Fast", "Schnell", "快速", "速い":
		return "Fast"
	default:
		return "Normal"
	}
}

// directionLabel says an Aura direction.
func directionLabel(c *ui.Context, dir string) string {
	switch dir {
	case "Left":
		return i18n.T(c, "aura.direction.left")
	case "Up":
		return i18n.T(c, "aura.direction.up")
	case "Down":
		return i18n.T(c, "aura.direction.down")
	default:
		return i18n.T(c, "aura.direction.right")
	}
}

// dirCode turns a translated direction back into asusd's name.
func dirCode(label string) string {
	switch label {
	case "Left", "Links", "向左", "左":
		return "Left"
	case "Up", "Oben", "向上", "上":
		return "Up"
	case "Down", "Unten", "向下", "下":
		return "Down"
	default:
		return "Right"
	}
}
