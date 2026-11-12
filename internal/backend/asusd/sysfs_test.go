// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// withSysfs points the fallback at a throwaway tree so the tests never
// touch the running machine.
func withSysfs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sysRoot = dir
	procRoot = filepath.Join(dir, "proc")
	stateRoot = filepath.Join(dir, "state")
	gpuSettle = 0
	t.Cleanup(func() {
		sysRoot, procRoot, stateRoot = "/sys", "/proc", ""
		gpuSettle = 500 * time.Millisecond
	})
	return dir
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setBoot(t *testing.T, id string) {
	t.Helper()
	writeTestFile(t, filepath.Join(procRoot, "sys/kernel/random/boot_id"), id+"\n")
}

func gpuFiles(t *testing.T, mux, dgpu int) (muxPath, dgpuPath string) {
	t.Helper()
	base := filepath.Join(sysRoot, "devices/platform/asus-nb-wmi")
	muxPath = filepath.Join(base, "gpu_mux_mode")
	dgpuPath = filepath.Join(base, "dgpu_disable")
	writeTestFile(t, muxPath, itoa(mux))
	writeTestFile(t, dgpuPath, itoa(dgpu))
	return
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	return "1"
}

func TestSysfsUltimateThenIntegratedNeedsTwoBoots(t *testing.T) {
	withSysfs(t)
	setBoot(t, "boot-a")
	muxPath, dgpuPath := gpuFiles(t, 1, 0)

	if err := setSysfsGPUMode(backend.GPUModeUltimate); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, muxPath); got != "0" {
		t.Fatalf("gpu_mux_mode = %q, want 0", got)
	}
	st := New().sysfsGPUState()
	if st.Mode != backend.GPUModeHybrid || st.Pending != backend.GPUModeUltimate || !st.Reboot {
		t.Fatalf("same-boot state is %+v, want hybrid pending ultimate", st)
	}

	// Reboot: the MUX is now 0, so reconcile drops the completed plan.
	setBoot(t, "boot-b")
	writeTestFile(t, muxPath, "0")
	st = New().sysfsGPUState()
	if st.Mode != backend.GPUModeUltimate || st.Pending != "" || st.Reboot {
		t.Fatalf("post-reboot state is %+v, want ultimate without pending", st)
	}

	// Leaving Ultimate may only move the MUX in this boot.
	if err := setSysfsGPUMode(backend.GPUModeIntegrated); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, muxPath); got != "1" {
		t.Fatalf("gpu_mux_mode = %q, want 1", got)
	}
	if got := readTestFile(t, dgpuPath); got != "0" {
		t.Fatalf("dgpu_disable = %q before reboot, want 0 (deferred)", got)
	}
	st = New().sysfsGPUState()
	if st.Mode != backend.GPUModeUltimate || st.Pending != backend.GPUModeIntegrated || !st.Reboot {
		t.Fatalf("leaving Ultimate state is %+v", st)
	}

	// After the reboot the panel is on the iGPU, so eco can be applied.
	setBoot(t, "boot-c")
	st = New().sysfsGPUState()
	if st.Mode != backend.GPUModeIntegrated || st.Pending != "" || st.Reboot {
		t.Fatalf("second-boot state is %+v, want integrated", st)
	}
	if got := readTestFile(t, dgpuPath); got != "1" {
		t.Fatalf("dgpu_disable = %q after reconcile, want 1", got)
	}
}

func TestSysfsCancelMuxLatchRestoresDisplayMode(t *testing.T) {
	withSysfs(t)
	setBoot(t, "boot-a")
	muxPath, _ := gpuFiles(t, 1, 0)

	if err := setSysfsGPUMode(backend.GPUModeUltimate); err != nil {
		t.Fatal(err)
	}
	// sysfs already reads 0, but the display is still hybrid. Cancelling
	// must write the mode the user is looking at, not the readback.
	if err := setSysfsGPUMode(backend.GPUModeHybrid); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, muxPath); got != "1" {
		t.Fatalf("gpu_mux_mode = %q after cancel, want 1", got)
	}
	st := New().sysfsGPUState()
	if st.Mode != backend.GPUModeHybrid || st.Pending != "" || st.Reboot {
		t.Fatalf("state after cancel is %+v", st)
	}
}

func TestSysfsRefusesIntegratedWhileDriverActive(t *testing.T) {
	withSysfs(t)
	setBoot(t, "boot-a")
	_, dgpuPath := gpuFiles(t, 1, 0)
	writeTestFile(t, filepath.Join(sysRoot, "module/nvidia_drm/refcnt"), "1\n")

	if err := setSysfsGPUMode(backend.GPUModeIntegrated); err == nil {
		t.Fatal("disabling the dGPU while its driver is active was allowed")
	}
	if got := readTestFile(t, dgpuPath); got != "0" {
		t.Fatalf("dgpu_disable = %q, want 0", got)
	}
}

func TestSysfsProfileAndChargeFallback(t *testing.T) {
	withSysfs(t)
	writeTestFile(t, filepath.Join(sysRoot, "firmware/acpi/platform_profile"), "balanced\n")
	writeTestFile(t, filepath.Join(sysRoot, "firmware/acpi/platform_profile_choices"), "quiet balanced performance\n")
	battery := filepath.Join(sysRoot, "class/power_supply/BAT0")
	writeTestFile(t, filepath.Join(battery, "charge_control_end_threshold"), "80\n")

	profile, choices := sysfsProfileState()
	if profile != backend.ProfileBalanced || len(choices) != 3 {
		t.Fatalf("profile state is %q %v", profile, choices)
	}
	if err := setSysfsProfile(backend.ProfilePerformance); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(sysRoot, "firmware/acpi/platform_profile")); got != "performance" {
		t.Fatalf("platform_profile = %q", got)
	}
	limit, ok := sysfsChargeLimit()
	if !ok || limit != 80 {
		t.Fatalf("charge limit is %d, %v", limit, ok)
	}
	if err := setSysfsChargeLimit(60); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(battery, "charge_control_end_threshold")); got != "60" {
		t.Fatalf("charge limit file = %q", got)
	}
}

func TestSysfsCPUBoostAndFirmwareFeature(t *testing.T) {
	withSysfs(t)
	noTurbo := filepath.Join(sysRoot, "devices/system/cpu/intel_pstate/no_turbo")
	writeTestFile(t, noTurbo, "0\n")

	f, ok := sysfsCPUBoostFeature()
	if !ok || f.Value != 1 {
		t.Fatalf("CPU boost feature is %+v, %v", f, ok)
	}
	if err := setSysfsCPUBoost(0); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, noTurbo); got != "1" {
		t.Fatalf("no_turbo = %q", got)
	}

	attr := filepath.Join(sysRoot, "class/firmware-attributes/asus-armoury/attributes/ppt_pl1_spl")
	writeTestFile(t, filepath.Join(attr, "current_value"), "45\n")
	writeTestFile(t, filepath.Join(attr, "min_value"), "25\n")
	writeTestFile(t, filepath.Join(attr, "max_value"), "60\n")
	writeTestFile(t, filepath.Join(attr, "scalar_increment"), "1\n")
	f, ok = sysfsFeature("ppt_pl1_spl")
	if !ok || f.Value != 45 || f.Min != 25 || f.Max != 60 || f.Step != 1 {
		t.Fatalf("PL1 feature is %+v, %v", f, ok)
	}
}

func TestMergeSysfsFillsAnEmptySnapshot(t *testing.T) {
	withSysfs(t)
	setBoot(t, "boot-a")
	gpuFiles(t, 1, 0)
	writeTestFile(t, filepath.Join(sysRoot, "firmware/acpi/platform_profile"), "performance\n")
	writeTestFile(t, filepath.Join(sysRoot, "firmware/acpi/platform_profile_choices"), "quiet balanced performance\n")

	snap := backend.Snapshot{Features: map[string]backend.Feature{}}
	New().mergeSysfs(&snap)
	if snap.Profile != backend.ProfilePerformance || len(snap.ProfileChoices) != 3 {
		t.Fatalf("profile merge gave %q %v", snap.Profile, snap.ProfileChoices)
	}
	if !snap.GPU.Supported || snap.GPU.Mode != backend.GPUModeHybrid {
		t.Fatalf("GPU merge gave %+v", snap.GPU)
	}
}
