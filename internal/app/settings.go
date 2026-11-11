// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ELY-Open-Source/CrychicUI/sound"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

const settingsFileName = "settings.json"

// settings is what the app keeps between runs.
type settings struct {
	PPDBindings map[string]string `json:"ppd_bindings,omitempty"`
	Muted       *bool             `json:"muted,omitempty"`
}

// defaultPPDBindings ties the usual firmware modes to the PPD presets
// they most closely match. Users can change or clear each binding in the
// performance page.
func defaultPPDBindings() map[backend.Profile]backend.PPDPreset {
	return map[backend.Profile]backend.PPDPreset{
		backend.ProfileQuiet:       backend.PPDPowerSaver,
		backend.ProfileBalanced:    backend.PPDBalanced,
		backend.ProfilePerformance: backend.PPDPerformance,
		backend.ProfileLowPower:    backend.PPDPowerSaver,
		backend.ProfileCustom:      backend.PPDBalanced,
	}
}

// settingsPath is the per-user settings file. It is empty when the
// platform cannot name a config directory, which turns persistence off.
func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "zephyr-center", settingsFileName)
}

// loadSettings merges the saved bindings over the defaults and restores
// the sound state. A stored empty preset is meaningful: it says to leave
// PPD alone for that mode.
func (a *App) loadSettings() {
	if a.settingsPath == "" {
		return
	}
	body, err := os.ReadFile(a.settingsPath)
	if err != nil {
		return
	}
	var saved settings
	if err := json.Unmarshal(body, &saved); err != nil {
		return
	}
	for name, value := range saved.PPDBindings {
		mode := backend.Profile(name)
		if !knownProfile(mode) {
			continue
		}
		preset := backend.PPDPreset(value)
		if preset != "" && !preset.Valid() {
			continue
		}
		a.ppdBindings[mode] = preset
	}
	if saved.Muted != nil {
		sound.SetEnabled(!*saved.Muted)
	}
}

// saveSettings writes the bindings and the current sound state. Errors
// are deliberately quiet: a read-only config directory should not stop a
// hardware setting.
func (a *App) saveSettings(bindings map[backend.Profile]backend.PPDPreset) {
	if a.settingsPath == "" {
		return
	}
	muted := !sound.Enabled()
	saved := settings{
		PPDBindings: make(map[string]string, len(bindings)),
		Muted:       &muted,
	}
	for mode, preset := range bindings {
		saved.PPDBindings[string(mode)] = string(preset)
	}
	body, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(a.settingsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, a.settingsPath); err != nil {
		_ = os.Remove(tmp)
	}
}

// saveCurrentSettings writes the settings after a change that only
// touches the sound state.
func (a *App) saveCurrentSettings() {
	a.mu.Lock()
	bindings := make(map[backend.Profile]backend.PPDPreset, len(a.ppdBindings))
	for mode, preset := range a.ppdBindings {
		bindings[mode] = preset
	}
	a.mu.Unlock()
	a.saveSettings(bindings)
}

// setSoundMuted changes the app's sound effects and saves the choice.
func (a *App) setSoundMuted(muted bool) {
	sound.SetEnabled(!muted)
	a.saveCurrentSettings()
	if !muted {
		sound.Play(sound.Toggle)
	}
	a.invalidateWindow()
}

func knownProfile(p backend.Profile) bool {
	for _, q := range backend.Profiles {
		if p == q {
			return true
		}
	}
	return false
}

// ppdBindingFor returns the preset bound to p, or "" when changing p
// should leave PPD alone.
func (a *App) ppdBindingFor(p backend.Profile) backend.PPDPreset {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ppdBindings[p]
}

// setPPDBinding records a binding and, when it is for the mode in use,
// switches PPD to it now.
func (a *App) setPPDBinding(p backend.Profile, preset backend.PPDPreset) {
	a.mu.Lock()
	if a.ppdBindings == nil {
		a.ppdBindings = map[backend.Profile]backend.PPDPreset{}
	}
	a.ppdBindings[p] = preset
	bindings := make(map[backend.Profile]backend.PPDPreset, len(a.ppdBindings))
	for mode, bound := range a.ppdBindings {
		bindings[mode] = bound
	}
	a.mu.Unlock()
	a.saveSettings(bindings)

	s := a.Snapshot()
	if preset == "" || !s.PPD.Supported || s.Profile != p {
		return
	}
	a.do("performance.ppd_applied", func(ctx context.Context) error {
		return a.be.SetPPDPreset(ctx, preset)
	})
}
