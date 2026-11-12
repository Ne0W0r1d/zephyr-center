// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// The Linux kernel exposes ASUS firmware through two interfaces:
//
//	/sys/class/firmware-attributes/asus-armoury/attributes/<name>/current_value
//	/sys/devices/platform/asus-nb-wmi/<name>
//
// G-Helper Linux prefers the firmware-attributes interface and falls back
// to the legacy asus-nb-wmi attributes. Zephyr Center normally talks to
// asusd, but using the same sysfs fallback keeps the important switches
// usable when asusd is old, missing, or does not export AsusArmoury.
var (
	sysRoot   = "/sys"
	procRoot  = "/proc"
	stateRoot = ""

	// gpuSettle is the delay after waking the dGPU before the MUX write.
	// Tests set it to zero.
	gpuSettle = 500 * time.Millisecond
)

type sysfsAttrSpec struct {
	legacy   string
	firmware string
}

var sysfsAttrs = map[string]sysfsAttrSpec{
	"panel_overdrive":        {"panel_od", "panel_overdrive"},
	"panel_od":               {"panel_od", "panel_overdrive"},
	"boot_sound":             {"boot_sound", "boot_sound"},
	"dgpu_disable":           {"dgpu_disable", "dgpu_disable"},
	"gpu_mux_mode":           {"gpu_mux_mode", "gpu_mux_mode"},
	"ppt_pl1_spl":            {"ppt_pl1_spl", "ppt_pl1_spl"},
	"ppt_pl2_sppt":           {"ppt_pl2_sppt", "ppt_pl2_sppt"},
	"nv_dynamic_boost":       {"nv_dynamic_boost", "nv_dynamic_boost"},
	"nv_temp_target":         {"nv_temp_target", "nv_temp_target"},
	"mini_led_mode":          {"mini_led_mode", "mini_led_mode"},
	"panel_hd_mode":          {"panel_hd_mode", "panel_hd_mode"},
	"mcu_powersave":          {"mcu_powersave", "mcu_powersave"},
	"screen_auto_brightness": {"screen_auto_brightness", "screen_auto_brightness"},
	"charge_mode":            {"charge_mode", "charge_mode"},
}

// sysfsFeatureOrder is the set the fallback reports. The firmware may not
// implement all of them; sysfsFeature checks each path.
var sysfsFeatureOrder = []string{
	"panel_overdrive",
	"boot_sound",
	"dgpu_disable",
	"gpu_mux_mode",
	"charge_mode",
	"ppt_pl1_spl",
	"ppt_pl2_sppt",
	"nv_dynamic_boost",
	"nv_temp_target",
	"mini_led_mode",
	"panel_hd_mode",
	"mcu_powersave",
	"screen_auto_brightness",
}

var sysfsBoolAttrs = map[string]bool{
	"panel_overdrive":        true,
	"panel_od":               true,
	"boot_sound":             true,
	"dgpu_disable":           true,
	"gpu_mux_mode":           true,
	"mini_led_mode":          false,
	"panel_hd_mode":          true,
	"mcu_powersave":          true,
	"screen_auto_brightness": true,
}

var sysfsRanges = map[string][3]int{
	"ppt_pl1_spl":      {25, 90, 1},
	"ppt_pl2_sppt":     {25, 135, 1},
	"nv_dynamic_boost": {5, 25, 1},
	"nv_temp_target":   {75, 87, 1},
}

func sysfsPath(rel string) string {
	return filepath.Join(sysRoot, filepath.FromSlash(rel))
}

func readSysfsString(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func readSysfsInt(path string) (int, bool) {
	s, ok := readSysfsString(path)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func writeSysfsInt(path string, value int) error {
	if err := os.WriteFile(path, []byte(strconv.Itoa(value)), 0o644); err != nil {
		return fmt.Errorf("asusd: write %s: %w", path, err)
	}
	return nil
}

func writeSysfsString(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("asusd: write %s: %w", path, err)
	}
	return nil
}

// sysfsAttrPath resolves an attribute, preferring the firmware-attributes
// interface as G-Helper Linux does. The second result says whether the
// path is a firmware-attributes current_value file, whose sibling files
// carry the range.
func sysfsAttrPath(name string) (path string, firmware, ok bool) {
	spec, ok := sysfsAttrs[name]
	if !ok {
		return "", false, false
	}
	fw := sysfsPath("class/firmware-attributes/asus-armoury/attributes/" + spec.firmware + "/current_value")
	if _, err := os.Stat(fw); err == nil {
		return fw, true, true
	}
	legacy := sysfsPath("devices/platform/asus-nb-wmi/" + spec.legacy)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, false, true
	}
	return "", false, false
}

func sysfsAttrValue(name string) (int, bool) {
	path, _, ok := sysfsAttrPath(name)
	if !ok {
		return 0, false
	}
	return readSysfsInt(path)
}

// dgpuDriverActive reports whether the kernel driver still owns the
// discrete GPU. G-Helper Linux refuses dgpu_disable=1 in this state
// because the ACPI hot-removal can panic the kernel. The same guard is
// applied here before either the sysfs or the asusd path disables it.
func dgpuDriverActive() bool {
	if _, err := os.Stat(sysfsPath("module/nvidia_drm")); err == nil {
		refcnt, ok := readSysfsInt(sysfsPath("module/nvidia_drm/refcnt"))
		// An unreadable refcount is treated as active.
		if !ok || refcnt > 0 {
			return true
		}
	}
	if _, err := os.Stat(sysfsPath("module/amdgpu")); err != nil {
		return false
	}
	matches, _ := filepath.Glob(sysfsPath("bus/pci/devices/*"))
	for _, dir := range matches {
		vendor, _ := readSysfsString(filepath.Join(dir, "vendor"))
		if vendor != "0x1002" {
			continue
		}
		class, _ := readSysfsString(filepath.Join(dir, "class"))
		if !strings.HasPrefix(class, "0x0300") && !strings.HasPrefix(class, "0x0302") {
			continue
		}
		if boot, _ := readSysfsString(filepath.Join(dir, "boot_vga")); boot == "1" {
			continue
		}
		driver, err := os.Readlink(filepath.Join(dir, "driver"))
		if err != nil || filepath.Base(driver) != "amdgpu" {
			continue
		}
		if status, _ := readSysfsString(filepath.Join(dir, "power/runtime_status")); status != "suspended" {
			return true
		}
	}
	return false
}

func sysfsWriteAttr(name string, value int) error {
	path, _, ok := sysfsAttrPath(name)
	if !ok {
		return fmt.Errorf("asusd: the firmware exposes no %s attribute", name)
	}
	return writeSysfsInt(path, value)
}

// sysfsAvailable reports whether the ASUS WMI interface is present at all.
func sysfsAvailable() bool {
	for _, path := range []string{
		sysfsPath("devices/platform/asus-nb-wmi"),
		sysfsPath("class/firmware-attributes/asus-armoury"),
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// --- platform profile -------------------------------------------------

func profileFromSysfsName(name string) backend.Profile {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "quiet":
		return backend.ProfileQuiet
	case "performance":
		return backend.ProfilePerformance
	case "low-power", "low_power", "lowpower":
		return backend.ProfileLowPower
	case "custom":
		return backend.ProfileCustom
	default:
		return backend.ProfileBalanced
	}
}

func sysfsProfileName(p backend.Profile) string {
	switch p {
	case backend.ProfileQuiet:
		return "quiet"
	case backend.ProfilePerformance:
		return "performance"
	case backend.ProfileLowPower:
		return "low-power"
	case backend.ProfileCustom:
		return "custom"
	default:
		return "balanced"
	}
}

func profileFromThrottle(code int) backend.Profile {
	switch code {
	case 1:
		return backend.ProfilePerformance
	case 2:
		return backend.ProfileQuiet
	case 3:
		return backend.ProfileLowPower
	default:
		return backend.ProfileBalanced
	}
}

func throttleFromProfile(p backend.Profile) (int, error) {
	switch p {
	case backend.ProfilePerformance:
		return 1, nil
	case backend.ProfileQuiet:
		return 2, nil
	case backend.ProfileLowPower:
		return 3, nil
	case backend.ProfileBalanced:
		return 0, nil
	}
	return 0, fmt.Errorf("asusd: the throttle_thermal_policy interface cannot select %q", p)
}

// sysfsProfileState reads the profile and the choices the kernel exposes.
func sysfsProfileState() (backend.Profile, []backend.Profile) {
	var profile backend.Profile
	if name, ok := readSysfsString(sysfsPath("firmware/acpi/platform_profile")); ok {
		profile = profileFromSysfsName(name)
	} else if code, ok := readSysfsInt(sysfsPath("devices/platform/asus-nb-wmi/throttle_thermal_policy")); ok {
		profile = profileFromThrottle(code)
	} else {
		return "", nil
	}

	var choices []backend.Profile
	if names, ok := readSysfsString(sysfsPath("firmware/acpi/platform_profile_choices")); ok {
		for _, name := range strings.Fields(names) {
			choices = append(choices, profileFromSysfsName(name))
		}
	}
	if len(choices) == 0 {
		choices = []backend.Profile{backend.ProfileQuiet, backend.ProfileBalanced, backend.ProfilePerformance}
	}
	return profile, choices
}

func setSysfsProfile(p backend.Profile) error {
	path := sysfsPath("firmware/acpi/platform_profile")
	if _, err := os.Stat(path); err == nil {
		if err := writeSysfsString(path, sysfsProfileName(p)); err != nil {
			return err
		}
		got, _ := readSysfsString(path)
		if profileFromSysfsName(got) != p {
			return fmt.Errorf("asusd: the firmware kept profile %q instead of %q", got, sysfsProfileName(p))
		}
		return nil
	}

	code, err := throttleFromProfile(p)
	if err != nil {
		return err
	}
	path = sysfsPath("devices/platform/asus-nb-wmi/throttle_thermal_policy")
	if _, err := os.Stat(path); err != nil {
		return errors.New("asusd: the kernel exposes no platform profile")
	}
	if err := writeSysfsInt(path, code); err != nil {
		return err
	}
	got, _ := readSysfsInt(path)
	if profileFromThrottle(got) != p {
		return fmt.Errorf("asusd: the firmware kept profile code %d instead of %d", got, code)
	}
	return nil
}

// --- charge limit -----------------------------------------------------

func sysfsChargePath() string {
	matches, _ := filepath.Glob(sysfsPath("class/power_supply/BAT*"))
	for _, dir := range matches {
		path := filepath.Join(dir, "charge_control_end_threshold")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func sysfsChargeLimit() (int, bool) {
	path := sysfsChargePath()
	if path == "" {
		return 0, false
	}
	return readSysfsInt(path)
}

func setSysfsChargeLimit(limit int) error {
	path := sysfsChargePath()
	if path == "" {
		return errors.New("asusd: the battery exposes no charge_control_end_threshold")
	}
	if err := writeSysfsInt(path, limit); err != nil {
		return err
	}
	got, _ := readSysfsInt(path)
	if got != limit {
		return fmt.Errorf("asusd: the battery kept charge limit %d instead of %d", got, limit)
	}
	return nil
}

// --- firmware features ------------------------------------------------

func sysfsPossibleValues(dir string) []int {
	s, ok := readSysfsString(filepath.Join(dir, "possible_values"))
	if !ok || s == "" {
		return nil
	}
	var out []int
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		if n, err := strconv.Atoi(field); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func sysfsFeature(name string) (backend.Feature, bool) {
	if name == "cpu_boost" {
		return sysfsCPUBoostFeature()
	}
	path, fw, ok := sysfsAttrPath(name)
	if !ok {
		return backend.Feature{}, false
	}
	value, ok := readSysfsInt(path)
	if !ok {
		return backend.Feature{}, false
	}
	f := backend.Feature{Supported: true, Value: value, Min: -1, Max: -1, Step: -1, Queued: -1}
	if fw {
		dir := filepath.Dir(path)
		if n, ok := readSysfsInt(filepath.Join(dir, "min_value")); ok {
			f.Min = n
		}
		if n, ok := readSysfsInt(filepath.Join(dir, "max_value")); ok {
			f.Max = n
		}
		if n, ok := readSysfsInt(filepath.Join(dir, "scalar_increment")); ok {
			f.Step = n
		}
		if values := sysfsPossibleValues(dir); len(values) > 0 {
			f.Values = values
		}
	}
	if sysfsBoolAttrs[name] && len(f.Values) == 0 {
		f.Values = []int{0, 1}
	}
	if r, ok := sysfsRanges[name]; ok {
		if f.Min < 0 {
			f.Min = r[0]
		}
		if f.Max < 0 {
			f.Max = r[1]
		}
		if f.Step <= 0 {
			f.Step = r[2]
		}
	}
	return f, true
}

func sysfsCPUBoostFeature() (backend.Feature, bool) {
	path := sysfsPath("devices/system/cpu/cpufreq/boost")
	if value, ok := readSysfsInt(path); ok {
		return backend.Feature{Supported: true, Value: value, Min: -1, Max: -1, Step: -1, Values: []int{0, 1}, Queued: -1}, true
	}
	path = sysfsPath("devices/system/cpu/intel_pstate/no_turbo")
	if value, ok := readSysfsInt(path); ok {
		return backend.Feature{Supported: true, Value: 1 - value, Min: -1, Max: -1, Step: -1, Values: []int{0, 1}, Queued: -1}, true
	}
	return backend.Feature{}, false
}

func setSysfsCPUBoost(value int) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("asusd: CPU boost must be 0 or 1, not %d", value)
	}
	path := sysfsPath("devices/system/cpu/cpufreq/boost")
	if _, err := os.Stat(path); err == nil {
		if err := writeSysfsInt(path, value); err != nil {
			return err
		}
		got, _ := readSysfsInt(path)
		if got != value {
			return fmt.Errorf("asusd: the kernel kept CPU boost %d instead of %d", got, value)
		}
		return nil
	}
	path = sysfsPath("devices/system/cpu/intel_pstate/no_turbo")
	if _, err := os.Stat(path); err != nil {
		return errors.New("asusd: the kernel exposes no CPU boost control")
	}
	inverted := 1 - value
	if err := writeSysfsInt(path, inverted); err != nil {
		return err
	}
	got, _ := readSysfsInt(path)
	if got != inverted {
		return fmt.Errorf("asusd: the kernel kept no_turbo %d instead of %d", got, inverted)
	}
	return nil
}

func setSysfsFeature(name string, value int) error {
	if name == "cpu_boost" {
		return setSysfsCPUBoost(value)
	}
	return sysfsWriteAttr(name, value)
}

// --- graphics mode ----------------------------------------------------

func sysfsGPUValues() (mux, dgpu int, muxOK, dgpuOK bool) {
	mux, muxOK = sysfsAttrValue("gpu_mux_mode")
	dgpu, dgpuOK = sysfsAttrValue("dgpu_disable")
	return
}

func sysfsMode(mux, dgpu int, muxOK, dgpuOK bool) backend.GPUMode {
	return gpuModeOf(mux, dgpu, muxOK, dgpuOK)
}

func sysfsGPUModes(muxOK, dgpuOK bool) []backend.GPUMode {
	var modes []backend.GPUMode
	if dgpuOK {
		modes = append(modes, backend.GPUModeIntegrated)
	}
	modes = append(modes, backend.GPUModeHybrid)
	if muxOK {
		modes = append(modes, backend.GPUModeUltimate)
	}
	return modes
}

// reconcileSysfsGPUMarker finishes a two-boot switch after the MUX has
// moved, or drops a plan the firmware did not honour.
func reconcileSysfsGPUMarker(m *gpuMarker) {
	if m == nil || markerQueuedThisBoot(m) {
		return
	}
	mux, dgpu, muxOK, dgpuOK := sysfsGPUValues()
	if m.Phase == gpuPhaseMux && muxOK && mux == 1 {
		if m.Target == backend.GPUModeIntegrated && dgpuOK && dgpu == 0 {
			if dgpuDriverActive() {
				// Keep the marker so the switch can be retried after the
				// driver is released.
				return
			}
			if err := sysfsWriteAttr("dgpu_disable", 1); err != nil {
				return
			}
			time.Sleep(gpuSettle)
			if value, ok := sysfsAttrValue("dgpu_disable"); !ok || value != 1 {
				return
			}
		}
		clearGPUMarker()
		return
	}
	// The MUX stayed put or the plan was already complete. The hardware is
	// the authority after a boot, so never keep a stale pending mode.
	clearGPUMarker()
}

func (b *Backend) sysfsGPUState() backend.GPUState {
	var st backend.GPUState
	mux, dgpu, muxOK, dgpuOK := sysfsGPUValues()
	if !muxOK && !dgpuOK {
		return st
	}
	marker := readGPUMarker()
	reconcileSysfsGPUMarker(marker)
	mux, dgpu, muxOK, dgpuOK = sysfsGPUValues()

	st.Supported = true
	st.Mode = sysfsMode(mux, dgpu, muxOK, dgpuOK)
	st.Modes = sysfsGPUModes(muxOK, dgpuOK)

	if marker = readGPUMarker(); markerQueuedThisBoot(marker) {
		st.Mode = marker.Previous
		st.Pending = marker.Target
		st.Reboot = true
	} else if marker != nil {
		// Reconciliation could not finish (for example a permission error
		// or a dGPU that did not wake). Report the truth and keep the plan
		// visible so the user can retry after fixing permissions.
		st.Pending = marker.Target
		st.Reboot = marker.Target != st.Mode
	}
	return st
}

func setSysfsGPUMode(mode backend.GPUMode) error {
	mux, dgpu, muxOK, dgpuOK := sysfsGPUValues()
	if !muxOK && !dgpuOK {
		return errors.New("asusd: the kernel exposes no graphics switch")
	}

	previous := sysfsMode(mux, dgpu, muxOK, dgpuOK)
	if marker := readGPUMarker(); markerQueuedThisBoot(marker) {
		if marker.Target == mode {
			return nil
		}
		// sysfs already reads back the latched value, so restore the mode
		// the user is actually looking at, not the current readback.
		if err := applySysfsHardwareMode(marker.Previous, muxOK, dgpuOK); err != nil {
			return err
		}
		clearGPUMarker()
		mux, dgpu, muxOK, dgpuOK = sysfsGPUValues()
		previous = sysfsMode(mux, dgpu, muxOK, dgpuOK)
	}

	if previous == mode {
		return nil
	}

	switch mode {
	case backend.GPUModeUltimate:
		if !muxOK {
			return errors.New("asusd: this machine has no MUX to switch")
		}
		if mux == 0 {
			return nil
		}
		if dgpuOK && dgpu == 1 {
			if err := sysfsWriteAttr("dgpu_disable", 0); err != nil {
				return err
			}
			time.Sleep(gpuSettle)
			if value, ok := sysfsAttrValue("dgpu_disable"); ok && value == 1 {
				return errors.New("asusd: the discrete GPU is still off")
			}
		}
		if err := sysfsWriteAttr("gpu_mux_mode", 0); err != nil {
			return err
		}
		return writeGPUMarker(gpuMarker{
			BootID: currentBootID(), Target: mode,
			Phase: gpuPhaseComplete, Previous: previous,
		})

	case backend.GPUModeIntegrated, backend.GPUModeHybrid:
		// Writing dgpu_disable=1 while the MUX still points at the dGPU is
		// the black-screen state. Move the MUX first and finish the dGPU
		// write at the next boot, exactly as G-Helper Linux does.
		if muxOK && mux == 0 {
			if err := sysfsWriteAttr("gpu_mux_mode", 1); err != nil {
				return err
			}
			return writeGPUMarker(gpuMarker{
				BootID: currentBootID(), Target: mode,
				Phase: gpuPhaseMux, Previous: previous,
			})
		}
		if !dgpuOK {
			return errors.New("asusd: this machine cannot switch the discrete GPU")
		}
		target := 0
		if mode == backend.GPUModeIntegrated {
			target = 1
		}
		if dgpu != target {
			if target == 1 && dgpuDriverActive() {
				return errors.New("asusd: the discrete GPU driver is still active; close its users or let it suspend before switching to Integrated")
			}
			if err := sysfsWriteAttr("dgpu_disable", target); err != nil {
				return err
			}
		}
		clearGPUMarker()
		return nil
	}
	return fmt.Errorf("asusd: unknown graphics mode %q", mode)
}

// applySysfsHardwareMode restores the two firmware flags for a mode. The
// order is chosen so no intermediate state points the panel at a GPU that
// is powered off.
func applySysfsHardwareMode(mode backend.GPUMode, muxOK, dgpuOK bool) error {
	setMux := func(value int) error {
		if !muxOK {
			return nil
		}
		return sysfsWriteAttr("gpu_mux_mode", value)
	}
	setDgpu := func(value int) error {
		if !dgpuOK {
			return nil
		}
		return sysfsWriteAttr("dgpu_disable", value)
	}
	switch mode {
	case backend.GPUModeIntegrated:
		if err := setMux(1); err != nil {
			return err
		}
		return setDgpu(1)
	case backend.GPUModeUltimate:
		if err := setDgpu(0); err != nil {
			return err
		}
		return setMux(0)
	default:
		if err := setMux(1); err != nil {
			return err
		}
		return setDgpu(0)
	}
}

// --- snapshot merge ---------------------------------------------------

// mergeSysfs fills what asusd did not provide. It is deliberately
// additive: a working AsusArmoury property always wins.
func (b *Backend) mergeSysfs(snap *backend.Snapshot) {
	if snap.Profile == "" {
		if profile, choices := sysfsProfileState(); choices != nil {
			snap.Profile = profile
			snap.ProfileChoices = choices
		}
	}
	if len(snap.ProfileChoices) == 0 {
		if _, choices := sysfsProfileState(); len(choices) > 0 {
			snap.ProfileChoices = choices
		}
	}
	if !snap.ChargeSupported {
		if limit, ok := sysfsChargeLimit(); ok {
			snap.ChargeLimit = limit
			snap.ChargeSupported = true
		}
	}
	if !snap.GPU.Supported {
		snap.GPU = b.sysfsGPUState()
	}
	if snap.Features == nil {
		snap.Features = map[string]backend.Feature{}
	}
	for _, name := range sysfsFeatureOrder {
		if _, exists := snap.Features[name]; exists {
			continue
		}
		if f, ok := sysfsFeature(name); ok {
			snap.Features[name] = f
		}
	}
	if _, exists := snap.Features["cpu_boost"]; !exists {
		if f, ok := sysfsCPUBoostFeature(); ok {
			snap.Features["cpu_boost"] = f
		}
	}
}

// sysfsSnapshot is the no-asusd path. Sensors, battery and system info are
// already sysfs readers, so the same snapshot keeps working.
func (b *Backend) sysfsSnapshot(snap *backend.Snapshot) {
	snap.Backend = "sysfs"
	b.mergeSysfs(snap)
	if snap.Profile == "" {
		snap.Profile = backend.ProfileBalanced
	}
	if !sysfsAvailable() {
		snap.Err = "asusd is not running and the kernel exposes no ASUS WMI attributes"
	}
}
