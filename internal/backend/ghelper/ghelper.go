// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package ghelper drives the Windows bridge, a windowless program built on
// G-Helper's AsusACPI layer (windows/bridge, GPL-3.0-or-later). The bridge
// answers one JSON request per line on its standard input, so this package
// speaks the same protocol over a child process.
package ghelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// Backend is the bridge's client.
type Backend struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	path string
}

// New returns a backend that runs the bridge from helperPath, or from
// beside the app when helperPath is empty.
func New(helperPath string) *Backend {
	return &Backend{path: helperPath}
}

// Name implements backend.Backend.
func (b *Backend) Name() string { return "ghelper" }

// helperPath finds the bridge: the path given, else zephyr-center-helper.exe
// beside the executable.
func (b *Backend) helperPath() (string, error) {
	if b.path != "" {
		return b.path, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "zephyr-center-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

// start launches the bridge if it is not running.
func (b *Backend) start() error {
	if b.cmd != nil && b.cmd.ProcessState == nil {
		return nil
	}
	path, err := b.helperPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(path)
	hideWindow(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ghelper: start %s: %w", path, err)
	}
	b.cmd, b.in, b.out = cmd, in, bufio.NewReader(out)
	return nil
}

// request is one line of the bridge's protocol.
type request struct {
	Op        string `json:"op"`
	Name      string `json:"name,omitempty"`
	Value     int    `json:"value,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Fan       string `json:"fan,omitempty"`
	Temp      []int  `json:"temp,omitempty"`
	PWM       []int  `json:"pwm,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	Mode      uint32 `json:"mode,omitempty"`
	Zone      uint32 `json:"zone,omitempty"`
	Color1    []int  `json:"color1,omitempty"`
	Color2    []int  `json:"color2,omitempty"`
	Speed     string `json:"speed,omitempty"`
	Direction string `json:"direction,omitempty"`
	Boot      bool   `json:"boot,omitempty"`
	Awake     bool   `json:"awake,omitempty"`
	Sleep     bool   `json:"sleep,omitempty"`
	Shutdown  bool   `json:"shutdown,omitempty"`
}

// response is the bridge's reply.
type response struct {
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
	Backend       string    `json:"backend,omitempty"`
	Version       string    `json:"version,omitempty"`
	DaemonVersion string    `json:"daemon_version,omitempty"`
	Snapshot      *wireSnap `json:"snapshot,omitempty"`
}

// wireSnap mirrors the bridge's Snapshot.
type wireSnap struct {
	Connected       bool                   `json:"connected"`
	Model           string                 `json:"model"`
	Board           string                 `json:"board"`
	BIOS            string                 `json:"bios"`
	BIOSDate        string                 `json:"bios_date"`
	CPU             string                 `json:"cpu"`
	IGPU            string                 `json:"igpu"`
	DGPU            string                 `json:"dgpu"`
	Kernel          string                 `json:"kernel"`
	OS              string                 `json:"os"`
	Profile         string                 `json:"profile"`
	ProfileChoices  []string               `json:"profile_choices"`
	ChargeLimit     int                    `json:"charge_limit"`
	ChargeSupported bool                   `json:"charge_supported"`
	Sensors         wireSensors            `json:"sensors"`
	Battery         wireBattery            `json:"battery"`
	GPU             wireGPU                `json:"gpu"`
	Aura            wireAura               `json:"aura"`
	Features        map[string]wireFeature `json:"features"`
	Curves          map[string][]wireCurve `json:"curves"`
}

type wireSensors struct {
	CPUTemp float64 `json:"cpu_temp"`
	GPUTemp float64 `json:"gpu_temp"`
	CPURPM  int     `json:"cpu_rpm"`
	GPURPM  int     `json:"gpu_rpm"`
	MidRPM  int     `json:"mid_rpm"`
}

type wireBattery struct {
	Present  bool    `json:"present"`
	Capacity int     `json:"capacity"`
	Status   string  `json:"status"`
	PowerW   float64 `json:"power_w"`
	Health   float64 `json:"health"`
	Cycles   int     `json:"cycles"`
	TimeLeft string  `json:"time_left"`
}

type wireGPU struct {
	Supported bool     `json:"supported"`
	Mode      string   `json:"mode"`
	Modes     []string `json:"modes"`
	Pending   string   `json:"pending"`
	Reboot    bool     `json:"reboot"`
}

type wireAura struct {
	Supported    bool         `json:"supported"`
	Modes        []uint32     `json:"modes"`
	Brightness   uint32       `json:"brightness"`
	Brightnesses []uint32     `json:"brightnesses"`
	Mode         wireAuraMode `json:"mode"`
	Power        []wireAuraOn `json:"power"`
	PowerZones   []uint32     `json:"power_zones"`
	Zones        []uint32     `json:"zones"`
	DeviceType   uint32       `json:"device_type"`
	Path         string       `json:"path"`
}

type wireAuraMode struct {
	Mode      uint32  `json:"mode"`
	Zone      uint32  `json:"zone"`
	Color1    []uint8 `json:"color1"`
	Color2    []uint8 `json:"color2"`
	Speed     string  `json:"speed"`
	Direction string  `json:"direction"`
}

type wireAuraOn struct {
	Zone     uint32 `json:"zone"`
	Boot     bool   `json:"boot"`
	Awake    bool   `json:"awake"`
	Sleep    bool   `json:"sleep"`
	Shutdown bool   `json:"shutdown"`
}

type wireFeature struct {
	Supported bool  `json:"supported"`
	Value     int   `json:"value"`
	Min       int   `json:"min"`
	Max       int   `json:"max"`
	Step      int   `json:"step"`
	Values    []int `json:"values"`
}

type wireCurve struct {
	Fan     string `json:"fan"`
	Temp    []int  `json:"temp"`
	PWM     []int  `json:"pwm"`
	Enabled bool   `json:"enabled"`
}

// call sends one request and reads one reply.
func (b *Backend) call(ctx context.Context, r request) (response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.start(); err != nil {
		return response{}, err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return response{}, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = deadline // the bridge answers at once; the pipe has no deadline
	}
	if _, err := b.in.Write(append(body, '\n')); err != nil {
		b.reset()
		return response{}, fmt.Errorf("ghelper: write: %w", err)
	}
	line, err := b.out.ReadBytes('\n')
	if err != nil {
		b.reset()
		return response{}, fmt.Errorf("ghelper: read: %w", err)
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return response{}, fmt.Errorf("ghelper: decode: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "the bridge refused the request"
		}
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// reset stops a bridge that failed, so the next call starts another.
func (b *Backend) reset() {
	if b.in != nil {
		_ = b.in.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_, _ = b.cmd.Process.Wait()
	}
	b.cmd, b.in, b.out = nil, nil, nil
}

// Snapshot implements backend.Backend.
func (b *Backend) Snapshot(ctx context.Context) (backend.Snapshot, error) {
	resp, err := b.call(ctx, request{Op: "snapshot"})
	if err != nil {
		return backend.Snapshot{Backend: "ghelper", Err: err.Error()}, nil
	}
	if resp.Snapshot == nil {
		return backend.Snapshot{Backend: "ghelper", Err: "the bridge sent no snapshot"}, nil
	}
	return convert(resp), nil
}

// convert maps the bridge's snapshot into the app's.
func convert(resp response) backend.Snapshot {
	w := resp.Snapshot
	s := backend.Snapshot{
		Backend:       "ghelper",
		Version:       resp.Version,
		DaemonVersion: resp.DaemonVersion,
		Info: backend.SystemInfo{
			Product: w.Model, Board: w.Board, BIOS: w.BIOS, BIOSDate: w.BIOSDate,
			CPU: w.CPU, IGPU: w.IGPU, DGPU: w.DGPU, Kernel: w.Kernel, OS: w.OS,
		},
		Profile:         backend.Profile(w.Profile),
		ChargeLimit:     w.ChargeLimit,
		ChargeSupported: w.ChargeSupported,
		Sensors: backend.Sensors{
			CPUTemp: w.Sensors.CPUTemp, GPUTemp: w.Sensors.GPUTemp,
			CPURPM: w.Sensors.CPURPM, GPURPM: w.Sensors.GPURPM, MidRPM: w.Sensors.MidRPM,
		},
		Battery: backend.Battery{
			Present: w.Battery.Present, Capacity: w.Battery.Capacity, Status: w.Battery.Status,
			PowerW: w.Battery.PowerW, Health: w.Battery.Health, Cycles: w.Battery.Cycles,
			TimeLeft: w.Battery.TimeLeft,
		},
		GPU: backend.GPUState{
			Supported: w.GPU.Supported, Mode: backend.GPUMode(w.GPU.Mode), Pending: backend.GPUMode(w.GPU.Pending),
			Reboot: w.GPU.Reboot,
		},
		Features: map[string]backend.Feature{},
		Curves:   map[backend.Profile][]backend.FanCurve{},
		Aura:     convertAura(w.Aura),
	}
	for _, p := range w.ProfileChoices {
		s.ProfileChoices = append(s.ProfileChoices, backend.Profile(p))
	}
	for _, m := range w.GPU.Modes {
		s.GPU.Modes = append(s.GPU.Modes, backend.GPUMode(m))
	}
	for name, f := range w.Features {
		s.Features[name] = backend.Feature{
			Supported: f.Supported, Value: f.Value, Min: f.Min, Max: f.Max, Step: f.Step, Values: f.Values,
			Queued: -1,
		}
	}
	for p, curves := range w.Curves {
		for _, c := range curves {
			if len(c.Temp) != 8 || len(c.PWM) != 8 {
				continue
			}
			fc := backend.FanCurve{Fan: c.Fan, Enabled: c.Enabled}
			for i := 0; i < 8; i++ {
				fc.Temp[i] = uint8(clampInt(c.Temp[i], 0, 255))
				fc.PWM[i] = uint8(clampInt(c.PWM[i], 0, 255))
			}
			s.Curves[backend.Profile(p)] = append(s.Curves[backend.Profile(p)], fc)
		}
	}
	return s
}

// clampInt keeps a wire number inside the range a byte can hold, so a
// stray value from the bridge cannot wrap around.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// convertAura maps the bridge's lighting state into the app's.
func convertAura(w wireAura) backend.AuraState {
	st := backend.AuraState{
		Supported:    w.Supported,
		Modes:        w.Modes,
		Brightness:   w.Brightness,
		Brightnesses: w.Brightnesses,
		Zones:        w.Zones,
		PowerZones:   w.PowerZones,
		DeviceType:   w.DeviceType,
		Path:         w.Path,
		Mode: backend.AuraMode{
			Mode:      w.Mode.Mode,
			Zone:      w.Mode.Zone,
			Color1:    rgb3(w.Mode.Color1),
			Color2:    rgb3(w.Mode.Color2),
			Speed:     w.Mode.Speed,
			Direction: w.Mode.Direction,
		},
	}
	for _, p := range w.Power {
		st.Power = append(st.Power, backend.AuraPower{
			Zone: p.Zone, Boot: p.Boot, Awake: p.Awake, Sleep: p.Sleep, Shutdown: p.Shutdown,
		})
	}
	return st
}

// rgb3 reads an RGB triplet, defaulting missing channels to zero.
func rgb3(v []uint8) [3]byte {
	var out [3]byte
	copy(out[:], v)
	return out
}

// SetProfile implements backend.Backend.
func (b *Backend) SetProfile(ctx context.Context, p backend.Profile) error {
	value := map[backend.Profile]int{
		backend.ProfileBalanced: 0, backend.ProfilePerformance: 1, backend.ProfileQuiet: 2,
	}[p]
	_, err := b.call(ctx, request{Op: "set_profile", Value: value})
	return err
}

// SetPPDPreset implements backend.Backend. PPD is a Linux service, so the
// Windows bridge leaves it alone.
func (b *Backend) SetPPDPreset(ctx context.Context, preset backend.PPDPreset) error {
	return errors.New("ghelper: power-profiles-daemon is not available on Windows")
}

// SetChargeLimit implements backend.Backend.
func (b *Backend) SetChargeLimit(ctx context.Context, limit int) error {
	_, err := b.call(ctx, request{Op: "set_charge_limit", Value: limit})
	return err
}

// SetFeature implements backend.Backend.
func (b *Backend) SetFeature(ctx context.Context, name string, value int) error {
	_, err := b.call(ctx, request{Op: "set_feature", Name: name, Value: value})
	return err
}

// SetFanCurve implements backend.Backend.
func (b *Backend) SetFanCurve(ctx context.Context, profile backend.Profile, c backend.FanCurve) error {
	_, err := b.call(ctx, request{
		Op: "set_fan_curve", Profile: string(profile), Fan: c.Fan,
		Temp: intPoints(c.Temp[:]), PWM: intPoints(c.PWM[:]), Enabled: c.Enabled,
	})
	return err
}

// SetFanCurvesEnabled implements backend.Backend.
func (b *Backend) SetFanCurvesEnabled(ctx context.Context, profile backend.Profile, enabled bool) error {
	_, err := b.call(ctx, request{Op: "set_fan_curves_enabled", Profile: string(profile), Enabled: enabled})
	return err
}

// ResetFanCurves implements backend.Backend.
func (b *Backend) ResetFanCurves(ctx context.Context, profile backend.Profile) error {
	_, err := b.call(ctx, request{Op: "reset_fan_curves", Profile: string(profile)})
	return err
}

// SetGPUMode implements backend.Backend.
func (b *Backend) SetGPUMode(ctx context.Context, mode backend.GPUMode) error {
	value := map[backend.GPUMode]int{
		backend.GPUModeIntegrated: 0, backend.GPUModeHybrid: 1, backend.GPUModeUltimate: 2,
	}[mode]
	_, err := b.call(ctx, request{Op: "set_gpu_mode", Value: value})
	return err
}

// SetAura implements backend.Backend.
func (b *Backend) SetAura(ctx context.Context, m backend.AuraMode) error {
	_, err := b.call(ctx, request{
		Op: "set_aura", Mode: m.Mode, Zone: m.Zone,
		Color1: intPoints(m.Color1[:]),
		Color2: intPoints(m.Color2[:]),
		Speed:  m.Speed, Direction: m.Direction,
	})
	return err
}

// intPoints widens byte points to the number arrays the bridge reads.
// A []uint8 would marshal as a base64 string, which the bridge's int[]
// fields cannot parse.
func intPoints(v []uint8) []int {
	out := make([]int, len(v))
	for i, b := range v {
		out[i] = int(b)
	}
	return out
}

// SetAuraBrightness implements backend.Backend.
func (b *Backend) SetAuraBrightness(ctx context.Context, v uint32) error {
	_, err := b.call(ctx, request{Op: "set_aura_brightness", Value: int(v)})
	return err
}

// SetAuraPower implements backend.Backend.
func (b *Backend) SetAuraPower(ctx context.Context, p backend.AuraPower) error {
	_, err := b.call(ctx, request{
		Op: "set_aura_power", Zone: p.Zone,
		Boot: p.Boot, Awake: p.Awake, Sleep: p.Sleep, Shutdown: p.Shutdown,
	})
	return err
}
