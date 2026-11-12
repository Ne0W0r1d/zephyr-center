// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// gpuMarker survives across the boot that moves the MUX. The kernel writes
// gpu_mux_mode immediately but only wires the panel at POST, so the value
// in sysfs is not the mode the user is looking at until the machine has
// restarted. G-Helper Linux calls the same idea a MUX latch.
type gpuMarker struct {
	BootID   string          `json:"boot_id"`
	Target   backend.GPUMode `json:"target"`
	Phase    string          `json:"phase"`
	Previous backend.GPUMode `json:"previous"`
}

const (
	gpuPhaseMux      = "mux"
	gpuPhaseComplete = "complete"
)

// gpuStatePath is the per-user file. A system-wide daemon would use
// /var/lib, but Zephyr Center runs as the logged-in user.
func gpuStatePath() string {
	if stateRoot != "" {
		return filepath.Join(stateRoot, "gpu-state.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "zephyr-center", "gpu-state.json")
}

func readGPUMarker() *gpuMarker {
	path := gpuStatePath()
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m gpuMarker
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	switch m.Target {
	case backend.GPUModeIntegrated, backend.GPUModeHybrid, backend.GPUModeUltimate:
	default:
		return nil
	}
	if m.Phase != gpuPhaseMux && m.Phase != gpuPhaseComplete {
		return nil
	}
	return &m
}

func writeGPUMarker(m gpuMarker) error {
	path := gpuStatePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clearGPUMarker() {
	path := gpuStatePath()
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// currentBootID identifies the running kernel, not the process, exactly as
// G-Helper Linux uses /proc/sys/kernel/random/boot_id for its latch.
func currentBootID() string {
	b, err := os.ReadFile(filepath.Join(procRoot, "sys/kernel/random/boot_id"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// markerQueuedThisBoot reports a plan written by this boot session.
func markerQueuedThisBoot(m *gpuMarker) bool {
	return m != nil && m.BootID != "" && m.BootID == currentBootID()
}
