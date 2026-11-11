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

	auraMode      uint32
	auraZone      uint32
	auraSpeed     string
	auraDirection string
	auraColor1    ui.Color
	auraColor2    ui.Color
	auraDirty     bool
	auraSynced    bool
	auraPower     map[uint32]backend.AuraPower

	chargeDraft float64
	chargeSet   bool

	// ppdBindings maps each firmware profile to the PPD preset the app
	// switches to with it. An empty preset leaves PPD alone.
	ppdBindings  map[backend.Profile]backend.PPDPreset
	settingsPath string

	// localeChoice is the language the user picked, "" to follow the
	// desktop. The view reads c.Locale() for what is shown.
	localeChoice string
}

// New returns the app over be.
func New(be backend.Backend) *App {
	a := &App{
		be:           be,
		page:         "overview",
		fanPage:      string(backend.ProfileBalanced),
		fanFan:       "CPU",
		fanDraft:     map[string][8][2]float64{},
		fanEnable:    map[string]bool{},
		fanDrag:      map[string]int{},
		fanPoint:     map[string]int{},
		auraPower:    map[uint32]backend.AuraPower{},
		ppdBindings:  defaultPPDBindings(),
		settingsPath: settingsPath(),
		localeChoice: "",
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
	a.mu.Lock()
	a.profile, a.profileAt = &p, time.Now()
	a.mu.Unlock()
	preset := a.ppdBindingFor(p)
	ppdSupported := a.Snapshot().PPD.Supported
	a.do(p.Label(), func(ctx context.Context) error {
		if err := a.be.SetProfile(ctx, p); err != nil {
			return err
		}
		if ppdSupported && preset != "" {
			return a.be.SetPPDPreset(ctx, preset)
		}
		return nil
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

// gpuRestarts reports whether switching between two modes moves the MUX.
// Only the path through Ultimate does: the MUX is what makes a restart
// necessary, while the discrete GPU itself can be switched at runtime.
func gpuRestarts(from, to backend.GPUMode) bool {
	return from == backend.GPUModeUltimate || to == backend.GPUModeUltimate
}

// setFeature writes one firmware attribute.
func (a *App) setFeature(name string, value int) {
	a.do(name, func(ctx context.Context) error { return a.be.SetFeature(ctx, name, value) })
}
