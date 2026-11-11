// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package backend defines what the Zephyr Center UI needs from the
// machine, and the values both the Linux and the Windows backends report.
package backend

import (
	"context"
	"time"
)

// Profile is a platform performance profile.
type Profile string

// The profiles asusd and G-Helper know.
const (
	ProfileBalanced    Profile = "balanced"
	ProfilePerformance Profile = "performance"
	ProfileQuiet       Profile = "quiet"
	ProfileLowPower    Profile = "low-power"
	ProfileCustom      Profile = "custom"
)

// Profiles lists every profile in display order.
var Profiles = []Profile{ProfileQuiet, ProfileBalanced, ProfilePerformance, ProfileLowPower, ProfileCustom}

// Label is the English name of the profile, for catalogs that have no
// translation for it.
func (p Profile) Label() string {
	switch p {
	case ProfileQuiet:
		return "Quiet"
	case ProfilePerformance:
		return "Performance"
	case ProfileLowPower:
		return "Low power"
	case ProfileCustom:
		return "Custom"
	default:
		return "Balanced"
	}
}

// ProfileFromCode maps asusd's numeric profile to Profile.
func ProfileFromCode(code uint32) Profile {
	switch code {
	case 1:
		return ProfilePerformance
	case 2:
		return ProfileQuiet
	case 3:
		return ProfileLowPower
	case 4:
		return ProfileCustom
	default:
		return ProfileBalanced
	}
}

// Code is asusd's numeric profile.
func (p Profile) Code() uint32 {
	switch p {
	case ProfilePerformance:
		return 1
	case ProfileQuiet:
		return 2
	case ProfileLowPower:
		return 3
	case ProfileCustom:
		return 4
	default:
		return 0
	}
}

// GPUMode is the graphics mode the firmware exposes.
type GPUMode string

// The GPU modes asusd and G-Helper switch between.
const (
	GPUModeIntegrated GPUMode = "integrated"
	GPUModeHybrid     GPUMode = "hybrid"
	GPUModeUltimate   GPUMode = "ultimate"
)

// Label is the English name of the mode.
func (m GPUMode) Label() string {
	switch m {
	case GPUModeIntegrated:
		return "Integrated"
	case GPUModeUltimate:
		return "Ultimate"
	default:
		return "Hybrid"
	}
}

// AuraMode is one Aura lighting effect, as asusd's LedModeData carries it.
type AuraMode struct {
	Mode      uint32
	Zone      uint32
	Color1    [3]byte
	Color2    [3]byte
	Speed     string
	Direction string
}

// AuraPower is the on/off state of one power zone of the keyboard light.
type AuraPower struct {
	Zone     uint32
	Boot     bool
	Awake    bool
	Sleep    bool
	Shutdown bool
}

// AuraState is what the keyboard light supports and shows.
type AuraState struct {
	Supported        bool
	Modes            []uint32
	Brightness       uint32
	Brightnesses     []uint32
	Mode             AuraMode
	Power            []AuraPower
	PowerZones       []uint32
	Zones            []uint32
	DeviceType       uint32
	Path             string
	BrightnessLabels []string
}

// FanCurve is one fan's eight-point curve, as asusd stores it. PWM is 0-255
// and Temp is degrees Celsius.
type FanCurve struct {
	Fan     string
	PWM     [8]uint8
	Temp    [8]uint8
	Enabled bool
}

// Points returns the curve as temperature/PWM pairs.
func (c FanCurve) Points() [8][2]float64 {
	var out [8][2]float64
	for i := range out {
		out[i] = [2]float64{float64(c.Temp[i]), float64(c.PWM[i]) * 100 / 255}
	}
	return out
}

// SetPoints replaces the curve from temperature/PWM pairs, clamping them.
func (c *FanCurve) SetPoints(pts [8][2]float64) {
	for i, p := range pts {
		c.Temp[i] = uint8(clamp(p[0], 0, 100))
		c.PWM[i] = uint8(clamp(p[1], 0, 100) * 255 / 100)
	}
}

// Feature is a firmware attribute asusd exposes under
// xyz.ljones.AsusArmoury, such as panel_overdrive or ppt_pl1_spl.
type Feature struct {
	Supported bool
	Value     int
	Min       int
	Max       int
	Step      int
	Values    []int
	// Queued is a value the daemon has accepted but not yet written, as
	// asusd queues the GPU attributes until shutdown; -1 when none.
	Queued int
}

// Battery is the state of the battery.
type Battery struct {
	Present  bool
	Capacity int
	Status   string
	PowerW   float64
	Health   float64
	Cycles   int
	TimeLeft string
}

// Sensors are the live temperatures, fan speeds and CPU load.
type Sensors struct {
	CPUTemp  float64
	GPUTemp  float64
	CPURPM   int
	GPURPM   int
	MidRPM   int
	CPUUsage float64
}

// SystemInfo describes the machine.
type SystemInfo struct {
	Product  string
	Board    string
	BIOS     string
	BIOSDate string
	CPU      string
	IGPU     string
	DGPU     string
	Kernel   string
	OS       string
}

// Snapshot is everything the UI shows at one moment.
type Snapshot struct {
	Backend         string
	Version         string
	DaemonVersion   string
	Generation      uint64
	Time            time.Time
	Info            SystemInfo
	Battery         Battery
	Sensors         Sensors
	Profile         Profile
	ProfileChoices  []Profile
	ChargeLimit     int
	ChargeSupported bool
	GPU             GPUState
	Features        map[string]Feature
	Curves          map[Profile][]FanCurve
	Aura            AuraState
	Err             string
}

// GPUState is the graphics mode and what the hardware supports.
type GPUState struct {
	Supported bool
	Mode      GPUMode
	Modes     []GPUMode
	// Pending is the mode the daemon queued for the next boot; empty when
	// nothing is queued. A queued value can equal Mode, when the user
	// changed their mind before the daemon applied it.
	Pending GPUMode
	// Reboot tells that the queued change moves the MUX, which the
	// firmware only applies at boot.
	Reboot bool
}

// NeedsReboot reports whether a graphics change is waiting for a restart.
func (g GPUState) NeedsReboot() bool {
	return g.Reboot
}

// Backend talks to one machine: asusd on Linux, G-Helper on Windows.
type Backend interface {
	// Name names the backend for the About page and the window subtitle.
	Name() string
	// Snapshot reads the machine. It may be called every few seconds.
	Snapshot(ctx context.Context) (Snapshot, error)
	SetProfile(ctx context.Context, p Profile) error
	SetChargeLimit(ctx context.Context, limit int) error
	SetFeature(ctx context.Context, name string, value int) error
	SetAura(ctx context.Context, m AuraMode) error
	SetAuraBrightness(ctx context.Context, brightness uint32) error
	SetAuraPower(ctx context.Context, p AuraPower) error
	SetFanCurve(ctx context.Context, profile Profile, curve FanCurve) error
	SetFanCurvesEnabled(ctx context.Context, profile Profile, enabled bool) error
	ResetFanCurves(ctx context.Context, profile Profile) error
	SetGPUMode(ctx context.Context, mode GPUMode) error
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
