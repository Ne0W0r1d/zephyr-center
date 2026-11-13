// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package app builds the Zephyr Center window: the sidebar, the
// pages, and the background reads that keep them current.
package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// App is the window's state. The background loop writes snapshots, the
// view reads them, and actions send writes to the backend.
type App struct {
	be backend.Backend

	snap atomic.Pointer[backend.Snapshot]

	mu         sync.Mutex
	invalidate func()
	window     windowControl
	toastKey   string
	toastText  string
	profile    *backend.Profile
	profileAt  time.Time
	gpu        *backend.GPUState
	gpuAt      time.Time
	charge     *int
	chargeAt   time.Time
	// chargeOnce is the limit to put back after a one-time full charge,
	// or 0 when no such charge is waiting.
	chargeOnce int

	page      string
	fanPage   string
	fanFan    string
	fanDraft  map[string][8][2]float64
	fanEnable map[string]bool
	fanDrag   map[string]int
	fanPoint  map[string]int

	advancedOpen bool

	auraMode       uint32
	auraZone       uint32
	auraSpeed      string
	auraDirection  string
	auraColor1     ui.Color
	auraColor2     ui.Color
	auraBrightness uint32
	auraProfile    backend.Profile
	auraDirty      bool
	auraSynced     bool
	auraPower      map[uint32]backend.AuraPower

	chargeDraft float64
	chargeSet   bool

	// ppdBindings maps each firmware profile to the PPD preset the app
	// switches to with it. An empty preset leaves PPD alone.
	ppdBindings       map[backend.Profile]backend.PPDPreset
	powerModeBindings map[backend.Profile]int
	profileFeatures   map[backend.Profile]map[string]int
	profileAura       map[backend.Profile]auraPreset
	settingsPath      string

	// localeChoice is the language the user picked, "" to follow the
	// desktop. The view reads c.Locale() for what is shown.
	localeChoice string
}

// New returns the app over be.
func New(be backend.Backend) *App {
	a := &App{
		be:                be,
		page:              "overview",
		fanPage:           string(backend.ProfileBalanced),
		fanFan:            "CPU",
		fanDraft:          map[string][8][2]float64{},
		fanEnable:         map[string]bool{},
		fanDrag:           map[string]int{},
		fanPoint:          map[string]int{},
		auraPower:         map[uint32]backend.AuraPower{},
		ppdBindings:       defaultPPDBindings(),
		powerModeBindings: map[backend.Profile]int{},
		profileFeatures:   map[backend.Profile]map[string]int{},
		profileAura:       map[backend.Profile]auraPreset{},
		settingsPath:      settingsPath(),
		localeChoice:      "",
	}
	a.loadSettings()
	return a
}

// Backend returns the backend the app talks to.
func (a *App) Backend() backend.Backend { return a.be }

// Start reads the machine now and every two seconds after, so that the
// window never blocks on D-Bus or nvidia-smi.
func (a *App) Start() {
	go func() {
		a.refresh()
		a.applyStartupSettings()
		for {
			a.refresh()
			time.Sleep(2 * time.Second)
		}
	}()
}

// Refresh reads the machine once, synchronously, for tests and for the
// refresh button.
func (a *App) Refresh() { a.refresh() }

// refresh reads one snapshot and asks the window to draw again.
func (a *App) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	s, err := a.be.Snapshot(ctx)
	if err != nil {
		s.Err = err.Error()
	}
	a.restoreChargeOnce(&s)
	a.snap.Store(&s)
	a.mu.Lock()
	inv := a.invalidate
	a.mu.Unlock()
	if inv != nil {
		inv()
	}
}

// applyStartupSettings restores the values the user explicitly saved for
// the profile the machine booted in. It deliberately leaves PPD alone:
// that daemon has its own desktop policy, and an untouched default
// binding should not rewrite it on every launch.
func (a *App) applyStartupSettings() {
	s := a.Snapshot()
	if s.Profile == "" {
		return
	}
	features := a.featuresFor(s.Profile)
	aura, hasAura := a.auraFor(s.Profile)
	powerMode, powerBound := a.powerModeBindingFor(s.Profile)
	if len(features) == 0 && !hasAura && !powerBound {
		return
	}
	a.doTimeout(45*time.Second, "", func(ctx context.Context) error {
		var firstErr error
		if powerBound && hasFeature(s, "power_mode") &&
			s.Features["power_mode"].Value != powerMode {
			if err := a.be.SetFeature(ctx, "power_mode", powerMode); err != nil {
				firstErr = err
			}
		}
		for name, value := range features {
			if !rememberedFeature(name) || (powerBound && name == "power_mode") {
				continue
			}
			next, ok := profileFeatureValue(s, name, value)
			if !ok || s.Features[name].Value == next {
				continue
			}
			if err := a.be.SetFeature(ctx, name, next); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if hasAura && s.Aura.Supported && !sameAuraScene(s.Aura, aura) {
			if err := a.be.SetAura(ctx, aura.mode()); err != nil && firstErr == nil {
				firstErr = err
			}
			if err := a.be.SetAuraBrightness(ctx, aura.Brightness); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}

// sameAuraScene reports whether the hardware already shows the saved
// scene, so startup does not write the same lighting values again.
func sameAuraScene(s backend.AuraState, p auraPreset) bool {
	m := s.Mode
	return m.Mode == p.Mode && m.Zone == p.Zone &&
		m.Color1 == p.Color1 && m.Color2 == p.Color2 &&
		m.Speed == p.Speed && m.Direction == p.Direction &&
		s.Brightness == p.Brightness
}

// setInvalidate records how to ask the window for a frame.
func (a *App) setInvalidate(fn func()) {
	a.mu.Lock()
	a.invalidate = fn
	a.mu.Unlock()
}

// invalidate asks for a frame if the window has drawn once.
func (a *App) invalidateWindow() {
	a.mu.Lock()
	inv := a.invalidate
	a.mu.Unlock()
	if inv != nil {
		inv()
	}
}

// Snapshot returns the last read, with a user's just-made choice laid over
// it until the daemon reports it, so a control does not jump back.
func (a *App) Snapshot() backend.Snapshot {
	var s backend.Snapshot
	if p := a.snap.Load(); p != nil {
		s = *p
	} else {
		s = backend.Snapshot{Backend: a.be.Name(), Err: ""}
	}
	if s.Backend == "" {
		s.Backend = a.be.Name()
	}
	if s.Features == nil {
		s.Features = map[string]backend.Feature{}
	}
	if s.Curves == nil {
		s.Curves = map[backend.Profile][]backend.FanCurve{}
	}
	a.mu.Lock()
	if a.profile != nil && time.Since(a.profileAt) < 4*time.Second {
		s.Profile = *a.profile
	}
	if a.gpu != nil && time.Since(a.gpuAt) < 4*time.Second {
		s.GPU = *a.gpu
	}
	if a.charge != nil && time.Since(a.chargeAt) < 4*time.Second {
		s.ChargeLimit = *a.charge
	}
	a.mu.Unlock()
	return s
}

// SetProfile applies p. The tray's quick panel uses the same path as the
// performance page, so per-profile power and lighting settings follow.
func (a *App) SetProfile(p backend.Profile) { a.setProfile(p) }

// SetGPUMode applies mode. The quick panel and the graphics page share
// the same live switch and queued-MUX handling.
func (a *App) SetGPUMode(mode backend.GPUMode) { a.setGPU(mode) }

// toastLater shows the message of key in the window after the next action.
func (a *App) toastLater(key string) {
	a.mu.Lock()
	a.toastKey = key
	a.mu.Unlock()
	a.invalidateWindow()
}

// toastLaterText shows an already-formatted message, as a backend error.
func (a *App) toastLaterText(text string) {
	a.mu.Lock()
	a.toastText = text
	a.mu.Unlock()
	a.invalidateWindow()
}

// takeToast returns the toast to show, if any, and clears it. A key is
// said in the window's language, which only the view knows.
func (a *App) takeToast(locale string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	msg := a.toastText
	if a.toastKey != "" {
		msg = i18n.TLocale(locale, a.toastKey)
	}
	a.toastKey, a.toastText = "", ""
	return msg
}

// do runs one backend write, shows a toast on success and refreshes.
func (a *App) do(okMsg string, fn func(context.Context) error) {
	a.doTimeout(12*time.Second, okMsg, fn)
}

// doTimeout is do with a custom deadline, for firmware writes that wait
// on the hardware, such as powering the discrete GPU down.
func (a *App) doTimeout(timeout time.Duration, okMsg string, fn func(context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			a.toastLaterText(err.Error())
		} else if okMsg != "" {
			a.toastLater(okMsg)
		}
		a.refresh()
	}()
}

// setProfile applies p now, and keeps it in the window until asusd agrees.
func (a *App) setProfile(p backend.Profile) {
	before := a.Snapshot()
	if before.Profile != "" && before.Profile != p {
		a.captureProfileFeatures(before.Profile, before)
	}
	a.mu.Lock()
	a.profile, a.profileAt = &p, time.Now()
	a.mu.Unlock()

	s := a.Snapshot()
	preset := a.ppdBindingFor(p)
	ppdSupported := s.PPD.Supported
	powerMode, powerBound := a.powerModeBindingFor(p)
	features := a.featuresFor(p)
	aura, hasAura := a.auraFor(p)

	a.doTimeout(45*time.Second, "common.applied", func(ctx context.Context) error {
		if err := a.be.SetProfile(ctx, p); err != nil {
			return err
		}
		var firstErr error
		if ppdSupported && preset != "" {
			if err := a.be.SetPPDPreset(ctx, preset); err != nil {
				firstErr = err
			}
		}
		if powerBound && hasFeature(s, "power_mode") {
			if err := a.be.SetFeature(ctx, "power_mode", powerMode); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		for name, value := range features {
			if !rememberedFeature(name) {
				continue
			}
			if powerBound && name == "power_mode" {
				continue
			}
			next, ok := profileFeatureValue(s, name, value)
			if !ok {
				continue
			}
			if err := a.be.SetFeature(ctx, name, next); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if hasAura && s.Aura.Supported {
			if err := a.be.SetAura(ctx, aura.mode()); err != nil && firstErr == nil {
				firstErr = err
			}
			if err := a.be.SetAuraBrightness(ctx, aura.Brightness); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
}

// setCharge applies a charge limit.
func (a *App) setCharge(limit int) {
	a.mu.Lock()
	a.charge, a.chargeAt = &limit, time.Now()
	if limit != 100 {
		a.chargeOnce = 0
	}
	a.mu.Unlock()
	a.do("", func(ctx context.Context) error { return a.be.SetChargeLimit(ctx, limit) })
}

// chargeToFull lifts the charge limit to 100 once and remembers limit, so
// that refresh puts it back when the battery is full.
func (a *App) chargeToFull(limit int) {
	if limit >= 100 {
		return
	}
	a.mu.Lock()
	if a.chargeOnce == 0 {
		a.chargeOnce = limit
	}
	a.mu.Unlock()
	a.setCharge(100)
}

// chargeOnceLimit returns the limit a one-time full charge will restore,
// or 0 when none is pending.
func (a *App) chargeOnceLimit() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.chargeOnce
}

// restoreChargeOnce puts the saved limit back once the battery is full.
// It runs from refresh, before the snapshot reaches the view.
func (a *App) restoreChargeOnce(s *backend.Snapshot) {
	if !s.Battery.Present || s.Battery.Capacity < 100 {
		return
	}
	a.mu.Lock()
	prev := a.chargeOnce
	if prev > 0 {
		a.chargeOnce = 0
		a.charge, a.chargeAt = nil, time.Time{}
	}
	a.mu.Unlock()
	if prev == 0 {
		return
	}
	s.ChargeLimit = prev
	a.do("", func(ctx context.Context) error { return a.be.SetChargeLimit(ctx, prev) })
}

// setGPU applies a graphics mode. Switching the discrete GPU on or off is
// live; moving the MUX, to or from Ultimate, takes effect on the next boot.
func (a *App) setGPU(mode backend.GPUMode) {
	g := a.Snapshot().GPU
	reboot := gpuRestarts(g.Mode, mode)
	if reboot {
		g.Pending, g.Reboot = mode, true
	} else {
		g.Mode, g.Pending, g.Reboot = mode, "", false
	}
	a.mu.Lock()
	a.gpu, a.gpuAt = &g, time.Now()
	a.mu.Unlock()
	msg := "gpu.switched"
	if reboot {
		msg = "gpu.applied"
	}
	a.doTimeout(30*time.Second, msg, func(ctx context.Context) error { return a.be.SetGPUMode(ctx, mode) })
}

// restart reboots so a queued MUX change reaches POST. It is a real
// restart, not a shutdown: Windows Fast Startup can skip the firmware
// step that latches the new MUX value.
func (a *App) restart() {
	a.doTimeout(10*time.Second, "", func(ctx context.Context) error {
		return a.be.Restart(ctx)
	})
}

// gpuRestarts reports whether switching between two modes moves the MUX.
// Only the path through Ultimate does: the MUX is what makes a restart
// necessary, while the discrete GPU itself can be switched at runtime.
func gpuRestarts(from, to backend.GPUMode) bool {
	return from == backend.GPUModeUltimate || to == backend.GPUModeUltimate
}

// rememberedFeature reports whether a firmware value belongs to a
// performance profile. Case and lighting are global, and the GPU flags
// have their own switch path.
func rememberedFeature(name string) bool {
	switch name {
	case "panel_overdrive", "panel_od", "boot_sound", "charge_mode",
		"dgpu_disable", "gpu_mux_mode", "power_mode":
		return false
	default:
		return true
	}
}

// profileFeatureValue adapts a remembered value to what the firmware
// currently accepts, so a saved PL or boost value cannot be written past
// that model's range.
func profileFeatureValue(s backend.Snapshot, name string, value int) (int, bool) {
	f, ok := s.Features[name]
	if !ok || !f.Supported {
		return 0, false
	}
	if len(f.Values) > 0 {
		for _, allowed := range f.Values {
			if allowed == value {
				return value, true
			}
		}
		return 0, false
	}
	if f.Min >= 0 && value < f.Min {
		value = f.Min
	}
	if f.Max >= 0 && value > f.Max {
		value = f.Max
	}
	if f.Step > 1 && f.Min >= 0 {
		value = f.Min + (value-f.Min)/f.Step*f.Step
	}
	return value, true
}

// setFeature writes one firmware attribute.
func (a *App) setFeature(name string, value int) {
	if rememberedFeature(name) {
		a.rememberFeature(a.Snapshot().Profile, name, value)
	}
	a.do("common.applied", func(ctx context.Context) error { return a.be.SetFeature(ctx, name, value) })
}
