// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package fake is an in-memory backend with a plausible machine, for
// screenshots, tests and trying the app without asusd or ASUS hardware.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// Backend is a fake machine.
type Backend struct {
	mu   sync.Mutex
	snap backend.Snapshot
	gen  uint64
}

// New returns a Zephyrus-like machine at a quiet moment.
func New() *Backend {
	b := &Backend{}
	b.snap = backend.Snapshot{
		Backend:       "demo",
		Version:       "0.1.0",
		DaemonVersion: "6.5.0",
		Info: backend.SystemInfo{
			Product:  "ROG Zephyrus M16 GU603ZM",
			Board:    "GU603ZM",
			BIOS:     "GU603ZM.311",
			BIOSDate: "2022-12-22",
			CPU:      "12th Gen Intel(R) Core(TM) i7-12700H",
			IGPU:     "Intel Alder Lake-P Integrated Graphics",
			DGPU:     "NVIDIA GeForce RTX 3060 Laptop GPU",
			Kernel:   "7.3.3-300.fc45.x86_64",
			OS:       "Fedora Linux 45 (GNOME)",
		},
		Battery: backend.Battery{
			Present: true, Capacity: 62, Status: "Discharging", PowerW: 18.4,
			Health: 74.9, Cycles: 132, TimeLeft: "2h 12m",
		},
		Sensors: backend.Sensors{
			CPUTemp: 61, GPUTemp: 47, CPURPM: 3400, GPURPM: 3100, CPUUsage: 14,
		},
		Profile:         backend.ProfileBalanced,
		ProfileChoices:  []backend.Profile{backend.ProfileQuiet, backend.ProfileBalanced, backend.ProfilePerformance},
		ChargeLimit:     80,
		ChargeSupported: true,
		PPD: backend.PPDState{
			Supported: true,
			Active:    backend.PPDBalanced,
			Profiles:  append([]backend.PPDPreset(nil), backend.PPDPresets...),
		},
		GPU: backend.GPUState{
			Supported: true, Mode: backend.GPUModeHybrid,
			Modes: []backend.GPUMode{backend.GPUModeIntegrated, backend.GPUModeHybrid, backend.GPUModeUltimate},
		},
		Features: map[string]backend.Feature{
			"panel_overdrive":  {Supported: true, Value: 1, Queued: -1, Values: []int{0, 1}},
			"boot_sound":       {Supported: true, Value: 0, Queued: -1, Values: []int{0, 1}},
			"charge_mode":      {Supported: true, Value: 1, Queued: -1, Values: []int{0, 1, 2}},
			"dgpu_disable":     {Supported: true, Value: 0, Queued: -1, Values: []int{0, 1}},
			"gpu_mux_mode":     {Supported: true, Value: 1, Queued: -1, Values: []int{0, 1}},
			"ppt_pl1_spl":      {Supported: true, Value: 60, Queued: -1, Min: 25, Max: 90, Step: 1},
			"ppt_pl2_sppt":     {Supported: true, Value: 135, Queued: -1, Min: 25, Max: 135, Step: 1},
			"nv_dynamic_boost": {Supported: true, Value: 15, Queued: -1, Min: 5, Max: 20, Step: 1},
			"nv_temp_target":   {Supported: true, Value: 87, Queued: -1, Min: 75, Max: 87, Step: 1},
			"ppt_crossload":    {Supported: true, Value: 40, Queued: -1, Min: 0, Max: 40, Step: 1},
			"ppt_cpu_temp":     {Supported: true, Value: 95, Queued: -1, Min: 75, Max: 97, Step: 1},
			"cpu_boost":        {Supported: true, Value: 2, Queued: -1, Values: []int{0, 1, 2, 3, 4}},
			"power_mode":       {Supported: true, Value: 1, Queued: -1, Values: []int{0, 1, 2, 3}},
		},
		Aura: backend.AuraState{
			Supported:    true,
			Modes:        []uint32{0, 1, 2, 3, 10},
			Brightness:   2,
			Brightnesses: []uint32{0, 1, 2, 3},
			Mode: backend.AuraMode{
				Mode: 0, Zone: 0, Color1: [3]byte{227, 94, 242}, Color2: [3]byte{0, 120, 255},
				Speed: "Med", Direction: "Right",
			},
			Power:      []backend.AuraPower{{Zone: 1, Boot: true, Awake: true, Sleep: true, Shutdown: true}},
			PowerZones: []uint32{1},
			Zones:      []uint32{0, 1, 2, 3, 4},
		},
	}
	b.snap.Curves = map[backend.Profile][]backend.FanCurve{
		backend.ProfileQuiet:       {curve("CPU", quietCPU), curve("GPU", quietGPU)},
		backend.ProfileBalanced:    {curve("CPU", balancedCPU), curve("GPU", balancedGPU)},
		backend.ProfilePerformance: {curve("CPU", perfCPU), curve("GPU", perfGPU)},
	}
	return b
}

var (
	quietCPU    = [8][2]uint8{{30, 0}, {45, 20}, {55, 30}, {62, 40}, {70, 55}, {78, 70}, {86, 90}, {95, 100}}
	quietGPU    = [8][2]uint8{{30, 0}, {45, 18}, {55, 28}, {62, 38}, {70, 52}, {78, 68}, {86, 88}, {95, 100}}
	balancedCPU = [8][2]uint8{{30, 0}, {42, 22}, {52, 35}, {60, 48}, {68, 62}, {76, 78}, {84, 92}, {95, 100}}
	balancedGPU = [8][2]uint8{{30, 0}, {42, 20}, {52, 32}, {60, 45}, {68, 60}, {76, 75}, {84, 90}, {95, 100}}
	perfCPU     = [8][2]uint8{{30, 10}, {40, 32}, {50, 48}, {58, 62}, {66, 74}, {74, 86}, {82, 96}, {95, 100}}
	perfGPU     = [8][2]uint8{{30, 8}, {40, 30}, {50, 45}, {58, 60}, {66, 72}, {74, 84}, {82, 95}, {95, 100}}
)

// curve builds a fan curve from temperature/speed pairs.
func curve(fan string, pts [8][2]uint8) backend.FanCurve {
	c := backend.FanCurve{Fan: fan, Enabled: false}
	for i, p := range pts {
		c.Temp[i] = p[0]
		c.PWM[i] = uint8(float64(p[1]) * 255 / 100)
	}
	return c
}

// Name implements backend.Backend.
func (b *Backend) Name() string { return "demo" }

// Snapshot implements backend.Backend.
func (b *Backend) Snapshot(ctx context.Context) (backend.Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gen++
	s := b.snap
	s.Generation = b.gen
	s.Time = time.Now()
	return s, nil
}

// SetProfile implements backend.Backend.
func (b *Backend) SetProfile(ctx context.Context, p backend.Profile) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, q := range b.snap.ProfileChoices {
		if q == p {
			b.snap.Profile = p
			return nil
		}
	}
	return fmt.Errorf("fake: profile %q is not offered", p)
}

// SetPPDPreset implements backend.Backend.
func (b *Backend) SetPPDPreset(ctx context.Context, preset backend.PPDPreset) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.snap.PPD.Supported {
		return fmt.Errorf("fake: power-profiles-daemon is not available")
	}
	for _, p := range b.snap.PPD.Profiles {
		if p == preset {
			b.snap.PPD.Active = preset
			return nil
		}
	}
	return fmt.Errorf("fake: PPD preset %q is not offered", preset)
}

// SetChargeLimit implements backend.Backend.
func (b *Backend) SetChargeLimit(ctx context.Context, limit int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snap.ChargeLimit = limit
	return nil
}

// SetFeature implements backend.Backend.
func (b *Backend) SetFeature(ctx context.Context, name string, value int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, ok := b.snap.Features[name]
	if !ok {
		return fmt.Errorf("fake: no feature %q", name)
	}
	f.Value = value
	b.snap.Features[name] = f
	return nil
}

// SetAura implements backend.Backend.
func (b *Backend) SetAura(ctx context.Context, m backend.AuraMode) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snap.Aura.Mode = m
	return nil
}

// SetAuraBrightness implements backend.Backend.
func (b *Backend) SetAuraBrightness(ctx context.Context, v uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snap.Aura.Brightness = v
	return nil
}

// SetAuraPower implements backend.Backend.
func (b *Backend) SetAuraPower(ctx context.Context, p backend.AuraPower) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.snap.Aura.Power {
		if b.snap.Aura.Power[i].Zone == p.Zone {
			b.snap.Aura.Power[i] = p
		}
	}
	return nil
}

// SetFanCurve implements backend.Backend.
func (b *Backend) SetFanCurve(ctx context.Context, profile backend.Profile, c backend.FanCurve) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	curves := b.snap.Curves[profile]
	for i := range curves {
		if curves[i].Fan == c.Fan {
			curves[i] = c
		}
	}
	b.snap.Curves[profile] = curves
	return nil
}

// SetFanCurvesEnabled implements backend.Backend.
func (b *Backend) SetFanCurvesEnabled(ctx context.Context, profile backend.Profile, enabled bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	curves := b.snap.Curves[profile]
	for i := range curves {
		curves[i].Enabled = enabled
	}
	b.snap.Curves[profile] = curves
	return nil
}

// ResetFanCurves implements backend.Backend.
func (b *Backend) ResetFanCurves(ctx context.Context, profile backend.Profile) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snap.Curves[profile] = []backend.FanCurve{curve("CPU", balancedCPU), curve("GPU", balancedGPU)}
	return nil
}

// SetGPUMode implements backend.Backend.
func (b *Backend) SetGPUMode(ctx context.Context, m backend.GPUMode) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snap.GPU.Mode = m
	return nil
}
