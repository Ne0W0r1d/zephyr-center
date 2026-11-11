// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package asusd implements the Linux backend over asusd's D-Bus interface.
//
// asusd owns the hardware: profiles, the charge limit, fan curves, Aura
// lighting and the GPU mux all go through xyz.ljones.Asusd on the system
// bus. This package reads its properties and calls its methods, and reads
// the fans, temperatures and battery that asusd does not expose from
// sysfs. It never writes sysfs itself, so asusd stays the single writer.
package asusd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// Backend talks to asusd.
type Backend struct {
	mu      sync.Mutex
	conn    *dbus.Conn
	sensors *sensors
	gen     uint64

	auraPath string
	curves   map[backend.Profile][]backend.FanCurve
}

// New returns a backend, connecting to the system bus on the first call
// that needs it.
func New() *Backend {
	return &Backend{sensors: newSensors(), curves: map[backend.Profile][]backend.FanCurve{}}
}

// Name implements backend.Backend.
func (b *Backend) Name() string { return "asusd" }

// conn returns the system bus connection, opening it when needed.
func (b *Backend) connOrErr() (*dbus.Conn, error) {
	if b.conn != nil && b.conn.Connected() {
		return b.conn, nil
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("asusd: connect to the system bus: %w", err)
	}
	b.conn = conn
	return conn, nil
}

// root returns the platform object.
func (b *Backend) root() (dbus.BusObject, error) {
	conn, err := b.connOrErr()
	if err != nil {
		return nil, err
	}
	return conn.Object(service, rootPath), nil
}

// get reads a property, returning the error as D-Bus reported it.
func get(obj dbus.BusObject, iface, name string) (dbus.Variant, error) {
	v, err := obj.GetProperty(iface + "." + name)
	if err != nil {
		return dbus.Variant{}, err
	}
	return v, nil
}

// Snapshot implements backend.Backend.
func (b *Backend) Snapshot(ctx context.Context) (backend.Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gen++
	snap := backend.Snapshot{
		Backend:    "asusd",
		Generation: b.gen,
		Time:       time.Now(),
		Info:       systemInfo(),
		Battery:    battery(),
		Sensors:    b.sensors.read(),
		Features:   map[string]backend.Feature{},
		Curves:     map[backend.Profile][]backend.FanCurve{},
	}
	root, err := b.root()
	if err != nil {
		snap.Err = err.Error()
		return snap, nil
	}
	if v, err := get(root, platform, "Version"); err == nil {
		snap.DaemonVersion, _ = v.Value().(string)
	}
	if v, err := get(root, platform, "PlatformProfile"); err == nil {
		if code, ok := v.Value().(uint32); ok {
			snap.Profile = backend.ProfileFromCode(code)
		}
	}
	if v, err := get(root, platform, "PlatformProfileChoices"); err == nil {
		if codes, ok := v.Value().([]uint32); ok {
			for _, code := range codes {
				snap.ProfileChoices = append(snap.ProfileChoices, backend.ProfileFromCode(code))
			}
		}
	}
	if len(snap.ProfileChoices) == 0 {
		snap.ProfileChoices = []backend.Profile{backend.ProfileQuiet, backend.ProfileBalanced, backend.ProfilePerformance}
	}
	if v, err := get(root, platform, "ChargeControlEndThreshold"); err == nil {
		if n, ok := v.Value().(uint8); ok {
			snap.ChargeLimit = int(n)
			snap.ChargeSupported = true
		}
	}
	snap.GPU = b.gpuState(root)
	snap.Features = b.features(root)
	snap.Curves = b.readCurves(root, snap.ProfileChoices)
	snap.Aura = b.auraState(root)
	if snap.Profile == "" {
		snap.Profile = backend.ProfileBalanced
	}
	return snap, nil
}

// SetProfile implements backend.Backend.
func (b *Backend) SetProfile(ctx context.Context, p backend.Profile) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	return root.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, platform, "PlatformProfile", dbus.MakeVariant(p.Code())).Err
}

// SetChargeLimit implements backend.Backend.
func (b *Backend) SetChargeLimit(ctx context.Context, limit int) error {
	if limit < 20 || limit > 100 {
		return fmt.Errorf("asusd: charge limit %d is outside 20–100", limit)
	}
	root, err := b.root()
	if err != nil {
		return err
	}
	return root.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, platform, "ChargeControlEndThreshold", dbus.MakeVariant(uint8(limit))).Err
}

// armouryPath maps a feature name to its object under asus_armoury.
func armouryPath(name string) string {
	return filepath.Join(rootPath, "asus_armoury", name)
}

// SetFeature implements backend.Backend, writing one AsusArmoury
// attribute such as panel_overdrive.
func (b *Backend) SetFeature(ctx context.Context, name string, value int) error {
	conn, err := b.connOrErr()
	if err != nil {
		return err
	}
	obj := conn.Object(service, dbus.ObjectPath(armouryPath(name)))
	return obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, armoury, "CurrentValue", dbus.MakeVariant(int32(value))).Err
}

// auraObject finds the Aura device object, which asusd names after its USB
// IDs, and keeps its path.
func (b *Backend) auraObject(root dbus.BusObject) (dbus.BusObject, error) {
	conn, err := b.connOrErr()
	if err != nil {
		return nil, err
	}
	if b.auraPath != "" {
		return conn.Object(service, dbus.ObjectPath(b.auraPath)), nil
	}
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	manager := conn.Object(service, "/")
	if err := manager.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects); err != nil {
		return nil, fmt.Errorf("asusd: list objects: %w", err)
	}
	for path, ifaces := range objects {
		if _, ok := ifaces[auraIface]; ok {
			b.auraPath = string(path)
			return conn.Object(service, path), nil
		}
	}
	return nil, errors.New("asusd: no Aura device")
}

// auraState reads the Aura device's properties.
func (b *Backend) auraState(root dbus.BusObject) backend.AuraState {
	var st backend.AuraState
	obj, err := b.auraObject(root)
	if err != nil {
		return st
	}
	st.Supported = true
	st.Path = b.auraPath
	if v, err := get(obj, auraIface, "SupportedBasicModes"); err == nil {
		if modes, ok := v.Value().([]uint32); ok {
			st.Modes = modes
		}
	}
	if v, err := get(obj, auraIface, "SupportedBrightness"); err == nil {
		if values, ok := v.Value().([]uint32); ok {
			st.Brightnesses = values
		}
	}
	if v, err := get(obj, auraIface, "SupportedBasicZones"); err == nil {
		if zones, ok := v.Value().([]uint32); ok {
			st.Zones = zones
		}
	}
	if v, err := get(obj, auraIface, "SupportedPowerZones"); err == nil {
		if zones, ok := v.Value().([]uint32); ok {
			st.PowerZones = zones
		}
	}
	if v, err := get(obj, auraIface, "Brightness"); err == nil {
		if n, ok := v.Value().(uint32); ok {
			st.Brightness = n
		}
	}
	if v, err := get(obj, auraIface, "DeviceType"); err == nil {
		if n, ok := v.Value().(uint32); ok {
			st.DeviceType = n
		}
	}
	if v, err := get(obj, auraIface, "LedModeData"); err == nil {
		var w auraModeWire
		if err := dbus.Store([]any{v}, &w); err == nil {
			st.Mode = convertMode(w)
		}
	}
	if v, err := get(obj, auraIface, "LedPower"); err == nil {
		var w auraPowerWire
		if err := dbus.Store([]any{v}, &w); err == nil {
			for _, z := range w.Zones {
				st.Power = append(st.Power, backend.AuraPower{
					Zone: z.Zone, Boot: z.Boot, Awake: z.Awake, Sleep: z.Sleep, Shutdown: z.Shutdown,
				})
			}
		}
	}
	return st
}

// SetAura implements backend.Backend.
func (b *Backend) SetAura(ctx context.Context, m backend.AuraMode) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	obj, err := b.auraObject(root)
	if err != nil {
		return err
	}
	return obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, auraIface, "LedModeData", dbus.MakeVariant(auraModeBody(m))).Err
}

// SetAuraBrightness implements backend.Backend.
func (b *Backend) SetAuraBrightness(ctx context.Context, brightness uint32) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	obj, err := b.auraObject(root)
	if err != nil {
		return err
	}
	return obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, auraIface, "Brightness", dbus.MakeVariant(brightness)).Err
}

// SetAuraPower implements backend.Backend.
func (b *Backend) SetAuraPower(ctx context.Context, p backend.AuraPower) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	obj, err := b.auraObject(root)
	if err != nil {
		return err
	}
	current, err := get(obj, auraIface, "LedPower")
	if err != nil {
		return err
	}
	var w auraPowerWire
	if err := dbus.Store([]any{current}, &w); err != nil {
		return err
	}
	found := false
	for i := range w.Zones {
		if w.Zones[i].Zone == p.Zone {
			w.Zones[i].Boot, w.Zones[i].Awake = p.Boot, p.Awake
			w.Zones[i].Sleep, w.Zones[i].Shutdown = p.Sleep, p.Shutdown
			found = true
		}
	}
	if !found {
		w.Zones = append(w.Zones, struct {
			Zone     uint32
			Boot     bool
			Awake    bool
			Sleep    bool
			Shutdown bool
		}{p.Zone, p.Boot, p.Awake, p.Sleep, p.Shutdown})
	}
	return obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, auraIface, "LedPower", dbus.MakeVariant(w)).Err
}

// readCurves reads every profile's fan curves, keeping the last good copy
// when the daemon fails.
func (b *Backend) readCurves(root dbus.BusObject, profiles []backend.Profile) map[backend.Profile][]backend.FanCurve {
	out := map[backend.Profile][]backend.FanCurve{}
	for _, p := range profiles {
		var wires []curveWire
		err := root.Call(fanCurves+".FanCurveData", 0, p.Code()).Store(&wires)
		if err != nil {
			if old, ok := b.curves[p]; ok {
				out[p] = old
			}
			continue
		}
		curves := make([]backend.FanCurve, 0, len(wires))
		for _, w := range wires {
			c, err := convertCurve(w)
			if err != nil {
				continue
			}
			curves = append(curves, c)
		}
		out[p] = curves
	}
	b.curves = out
	return out
}

// SetFanCurve implements backend.Backend.
func (b *Backend) SetFanCurve(ctx context.Context, profile backend.Profile, curve backend.FanCurve) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	return root.CallWithContext(ctx, fanCurves+".SetFanCurve", 0, profile.Code(), curveBody(curve)).Err
}

// SetFanCurvesEnabled implements backend.Backend.
func (b *Backend) SetFanCurvesEnabled(ctx context.Context, profile backend.Profile, enabled bool) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	return root.CallWithContext(ctx, fanCurves+".SetFanCurvesEnabled", 0, profile.Code(), enabled).Err
}

// ResetFanCurves implements backend.Backend.
func (b *Backend) ResetFanCurves(ctx context.Context, profile backend.Profile) error {
	root, err := b.root()
	if err != nil {
		return err
	}
	return root.CallWithContext(ctx, fanCurves+".SetCurvesToDefaults", 0, profile.Code()).Err
}

// gpuState reads dgpu_disable and gpu_mux_mode, as rog-control-center's
// GPU page does: mux 0 is Ultimate, dgpu 1 is Integrated.
func (b *Backend) gpuState(root dbus.BusObject) backend.GPUState {
	var st backend.GPUState
	mux, muxOK := b.armouryValue(root, "gpu_mux_mode")
	dgpu, dgpuOK := b.armouryValue(root, "dgpu_disable")
	if !muxOK && !dgpuOK {
		return st
	}
	st.Supported = true
	st.Mode = gpuModeOf(mux, dgpu, muxOK, dgpuOK)
	if dgpuOK {
		st.Modes = append(st.Modes, backend.GPUModeIntegrated)
	}
	st.Modes = append(st.Modes, backend.GPUModeHybrid)
	if muxOK {
		st.Modes = append(st.Modes, backend.GPUModeUltimate)
	}
	st.Pending, st.Reboot = b.pendingGPUMode(root, mux, dgpu, muxOK, dgpuOK)
	return st
}

// gpuModeOf names the mode that mux and dgpu describe: mux 0 is Ultimate,
// dgpu 1 is Integrated.
func gpuModeOf(mux, dgpu int, muxOK, dgpuOK bool) backend.GPUMode {
	switch {
	case muxOK && mux == 0:
		return backend.GPUModeUltimate
	case dgpuOK && dgpu == 1:
		return backend.GPUModeIntegrated
	default:
		return backend.GPUModeHybrid
	}
}

// pendingGPUMode reads the mode asusd has queued for the next boot and
// whether applying it moves the MUX. asusd keeps a queued value per
// attribute, so the two attributes are read together: a queued value
// replaces the current one for that attribute.
func (b *Backend) pendingGPUMode(root dbus.BusObject, mux, dgpu int, muxOK, dgpuOK bool) (backend.GPUMode, bool) {
	queuedMux, muxQueued := b.armouryQueued(root, "gpu_mux_mode")
	queuedDgpu, dgpuQueued := b.armouryQueued(root, "dgpu_disable")
	if !muxQueued && !dgpuQueued {
		return "", false
	}
	reboot := muxOK && muxQueued && queuedMux != mux
	if muxQueued {
		mux = queuedMux
	}
	if dgpuQueued {
		dgpu = queuedDgpu
	}
	return gpuModeOf(mux, dgpu, muxOK, dgpuOK), reboot
}

// armouryQueued reads an attribute's queued value. asusd reports -1 when
// nothing is queued, which ok reports as false.
func (b *Backend) armouryQueued(root dbus.BusObject, name string) (int, bool) {
	conn, err := b.connOrErr()
	if err != nil {
		return 0, false
	}
	obj := conn.Object(service, dbus.ObjectPath(armouryPath(name)))
	v, err := obj.GetProperty(armoury + ".QueuedGpuValue")
	if err != nil {
		return 0, false
	}
	if n, ok := v.Value().(int32); ok && n >= 0 {
		return int(n), true
	}
	return 0, false
}

// SetGPUMode implements backend.Backend.
//
// asusd queues the GPU attributes instead of writing them at once, and
// applies the queue when it shuts down, so the firmware switches on the
// next boot. Disabling or enabling the discrete GPU is a live change, so
// when the MUX stays put the queue is applied right away; a MUX change is
// left for the next boot, and the UI tells the user to restart.
func (b *Backend) SetGPUMode(ctx context.Context, mode backend.GPUMode) error {
	var dgpu, mux int
	switch mode {
	case backend.GPUModeIntegrated:
		dgpu, mux = 1, 1
	case backend.GPUModeUltimate:
		dgpu, mux = 0, 0
	default:
		dgpu, mux = 0, 1
	}
	root, err := b.root()
	if err != nil {
		return err
	}
	currentMux, muxOK := b.armouryValue(root, "gpu_mux_mode")
	_, dgpuOK := b.armouryValue(root, "dgpu_disable")
	if dgpuOK {
		if err := b.SetFeature(ctx, "dgpu_disable", dgpu); err != nil {
			return err
		}
	}
	if muxOK {
		if err := b.SetFeature(ctx, "gpu_mux_mode", mux); err != nil {
			return err
		}
	}
	if dgpuOK && (!muxOK || currentMux == mux) {
		return b.applyQueuedGPUValue(ctx, "dgpu_disable")
	}
	return nil
}

// applyQueuedGPUValue applies one attribute's queued value now instead of
// at shutdown. Only the discrete GPU switch supports this; a MUX write
// changes the display path, which the firmware cannot do at runtime.
func (b *Backend) applyQueuedGPUValue(ctx context.Context, name string) error {
	conn, err := b.connOrErr()
	if err != nil {
		return err
	}
	obj := conn.Object(service, dbus.ObjectPath(armouryPath(name)))
	return obj.CallWithContext(ctx, armoury+".ApplyQueuedGpuValue", 0).Err
}

// armouryValue reads one AsusArmoury attribute's current value.
func (b *Backend) armouryValue(root dbus.BusObject, name string) (int, bool) {
	conn, err := b.connOrErr()
	if err != nil {
		return 0, false
	}
	obj := conn.Object(service, dbus.ObjectPath(armouryPath(name)))
	v, err := obj.GetProperty(armoury + ".CurrentValue")
	if err != nil {
		return 0, false
	}
	if n, ok := v.Value().(int32); ok {
		return int(n), true
	}
	return 0, false
}

// features lists the AsusArmoury attributes asusd exposes, with their
// ranges and choices.
func (b *Backend) features(root dbus.BusObject) map[string]backend.Feature {
	out := map[string]backend.Feature{}
	conn, err := b.connOrErr()
	if err != nil {
		return out
	}
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	manager := conn.Object(service, "/")
	if err := manager.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects); err != nil {
		return out
	}
	for path, ifaces := range objects {
		props, ok := ifaces[armoury]
		if !ok {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(string(path), rootPath+"/asus_armoury/"), "/")
		if name == "" {
			continue
		}
		f := backend.Feature{Min: -1, Max: -1, Step: -1}
		if v, ok := props["CurrentValue"]; ok {
			if n, ok := v.Value().(int32); ok {
				f.Value = int(n)
			}
		}
		if v, ok := props["MinValue"]; ok {
			if n, ok := v.Value().(int32); ok {
				f.Min = int(n)
			}
		}
		if v, ok := props["MaxValue"]; ok {
			if n, ok := v.Value().(int32); ok {
				f.Max = int(n)
			}
		}
		if v, ok := props["ScalarIncrement"]; ok {
			if n, ok := v.Value().(int32); ok {
				f.Step = int(n)
			}
		}
		if v, ok := props["PossibleValues"]; ok {
			if values, ok := v.Value().([]int32); ok {
				for _, n := range values {
					f.Values = append(f.Values, int(n))
				}
			}
		}
		f.Queued = -1
		if v, ok := props["QueuedGpuValue"]; ok {
			if n, ok := v.Value().(int32); ok {
				f.Queued = int(n)
			}
		}
		f.Supported = true
		out[name] = f
	}
	return out
}

// systemInfo reads the model, BIOS, CPU and GPUs from sysfs and lspci.
func systemInfo() backend.SystemInfo {
	info := backend.SystemInfo{
		Product:  readTrim("/sys/class/dmi/id/product_name"),
		Board:    readTrim("/sys/class/dmi/id/board_name"),
		BIOS:     readTrim("/sys/class/dmi/id/bios_version"),
		BIOSDate: readTrim("/sys/class/dmi/id/bios_date"),
		CPU:      cpuModel(),
		Kernel:   kernelRelease(),
		OS:       osRelease(),
	}
	info.IGPU, info.DGPU = gpuNames()
	return info
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// cpuModel reads the model name from /proc/cpuinfo.
func cpuModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "model name") {
			if i := strings.IndexByte(line, ':'); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

func kernelRelease() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// osRelease reads PRETTY_NAME from os-release.
func osRelease() string {
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}
	return ""
}

// gpuNames lists the integrated and discrete GPU names from lspci -mm.
//
// Each line is five quoted fields — class, vendor, device, subsystem
// vendor and subsystem device — so the device's own name is the third,
// and the subsystem's is not the GPU's.
func gpuNames() (integrated, discrete string) {
	out, err := exec.Command("lspci", "-mm", "-nn").Output()
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "vga compatible controller") && !strings.Contains(lower, "3d controller") && !strings.Contains(lower, "display controller") {
			continue
		}
		parts := strings.Split(line, `"`)
		if len(parts) < 6 {
			continue
		}
		name := strings.TrimSpace(shortVendor(parts[3]) + " " + stripPCIID(parts[5]))
		if name == "" {
			continue
		}
		if strings.Contains(lower, "nvidia") || strings.Contains(lower, "advanced micro devices") {
			if discrete == "" {
				discrete = name
			}
		} else if integrated == "" {
			integrated = name
		}
	}
	return integrated, discrete
}

// stripPCIID removes lspci's trailing " [10de:2520]" from a name.
func stripPCIID(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, " ["); i > 0 && strings.HasSuffix(s, "]") {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// shortVendor shortens lspci's vendor names.
func shortVendor(v string) string {
	v = strings.TrimSpace(v)
	switch {
	case strings.Contains(v, "Intel"):
		return "Intel"
	case strings.Contains(v, "NVIDIA"):
		return "NVIDIA"
	case strings.Contains(v, "Advanced Micro Devices") || strings.Contains(v, "ATI"):
		return "AMD"
	default:
		return strings.TrimSpace(strings.TrimSuffix(v, " Corporation"))
	}
}
