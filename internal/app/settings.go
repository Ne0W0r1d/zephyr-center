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
	PPDBindings       map[string]string         `json:"ppd_bindings,omitempty"`
	PowerModeBindings map[string]int            `json:"power_mode_bindings,omitempty"`
	ProfileFeatures   map[string]map[string]int `json:"profile_features,omitempty"`
	ProfileAura       map[string]auraPreset     `json:"profile_aura,omitempty"`
	Muted             *bool                     `json:"muted,omitempty"`
}

// auraPreset is one lighting scene kept per firmware profile. Brightness
// is part of the scene, so switching profiles restores the whole look.
type auraPreset struct {
	Mode       uint32  `json:"mode"`
	Zone       uint32  `json:"zone"`
	Color1     [3]byte `json:"color1"`
	Color2     [3]byte `json:"color2"`
	Speed      string  `json:"speed"`
	Direction  string  `json:"direction"`
	Brightness uint32  `json:"brightness"`
}

// mode converts the saved scene into the value the backend writes.
func (p auraPreset) mode() backend.AuraMode {
	return backend.AuraMode{
		Mode: p.Mode, Zone: p.Zone,
		Color1: p.Color1, Color2: p.Color2,
		Speed: p.Speed, Direction: p.Direction,
	}
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
	for name, value := range saved.PowerModeBindings {
		mode := backend.Profile(name)
		if !knownProfile(mode) || value < 0 {
			continue
		}
		a.powerModeBindings[mode] = value
	}
	for name, values := range saved.ProfileFeatures {
		mode := backend.Profile(name)
		if !knownProfile(mode) || len(values) == 0 {
			continue
		}
		copy := make(map[string]int, len(values))
		for feature, value := range values {
			if feature != "" && rememberedFeature(feature) {
				copy[feature] = value
			}
		}
		if len(copy) > 0 {
			a.profileFeatures[mode] = copy
		}
	}
	for name, preset := range saved.ProfileAura {
		mode := backend.Profile(name)
		if !knownProfile(mode) {
			continue
		}
		a.profileAura[mode] = preset
	}
	if saved.Muted != nil {
		sound.SetEnabled(!*saved.Muted)
	}
}

// saveSettings writes the bindings, profile memory and sound state. Errors
// are deliberately quiet: a read-only config directory should not stop a
// hardware setting.
func (a *App) saveSettings() {
	if a.settingsPath == "" {
		return
	}
	a.mu.Lock()
	bindings := make(map[backend.Profile]backend.PPDPreset, len(a.ppdBindings))
	for mode, preset := range a.ppdBindings {
		bindings[mode] = preset
	}
	powerModes := make(map[backend.Profile]int, len(a.powerModeBindings))
	for mode, value := range a.powerModeBindings {
		powerModes[mode] = value
	}
	features := make(map[backend.Profile]map[string]int, len(a.profileFeatures))
	for mode, values := range a.profileFeatures {
		copy := make(map[string]int, len(values))
		for name, value := range values {
			copy[name] = value
		}
		features[mode] = copy
	}
	aura := make(map[backend.Profile]auraPreset, len(a.profileAura))
	for mode, preset := range a.profileAura {
		aura[mode] = preset
	}
	a.mu.Unlock()

	muted := !sound.Enabled()
	saved := settings{
		PPDBindings:       make(map[string]string, len(bindings)),
		PowerModeBindings: make(map[string]int, len(powerModes)),
		ProfileFeatures:   make(map[string]map[string]int, len(features)),
		ProfileAura:       make(map[string]auraPreset, len(aura)),
		Muted:             &muted,
	}
	for mode, preset := range bindings {
		saved.PPDBindings[string(mode)] = string(preset)
	}
	for mode, value := range powerModes {
		saved.PowerModeBindings[string(mode)] = value
	}
	for mode, values := range features {
		saved.ProfileFeatures[string(mode)] = values
	}
	for mode, preset := range aura {
		saved.ProfileAura[string(mode)] = preset
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
	a.saveSettings()
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
	a.mu.Unlock()
	a.saveSettings()

	s := a.Snapshot()
	if preset == "" || !s.PPD.Supported || s.Profile != p {
		return
	}
	a.do("performance.ppd_applied", func(ctx context.Context) error {
		return a.be.SetPPDPreset(ctx, preset)
	})
}

// powerModeBindingFor returns the Windows power mode bound to p. The
// boolean distinguishes 0 (best efficiency) from "unbound".
func (a *App) powerModeBindingFor(p backend.Profile) (int, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	value, ok := a.powerModeBindings[p]
	return value, ok
}

// setPowerModeBinding records a Windows power mode for p and applies it
// when p is the mode in use.
func (a *App) setPowerModeBinding(p backend.Profile, value int) {
	a.mu.Lock()
	if a.powerModeBindings == nil {
		a.powerModeBindings = map[backend.Profile]int{}
	}
	if value < 0 {
		delete(a.powerModeBindings, p)
	} else {
		a.powerModeBindings[p] = value
	}
	a.mu.Unlock()
	a.saveSettings()

	s := a.Snapshot()
	if value < 0 || s.Profile != p || !hasFeature(s, "power_mode") {
		return
	}
	a.do("performance.power_binding_applied", func(ctx context.Context) error {
		return a.be.SetFeature(ctx, "power_mode", value)
	})
}

// rememberFeature keeps a firmware value under the profile that was in
// use when the user changed it.
func (a *App) rememberFeature(profile backend.Profile, name string, value int) {
	a.mu.Lock()
	a.rememberFeatureLocked(profile, name, value)
	a.mu.Unlock()
	a.saveSettings()
}

func (a *App) rememberFeatureLocked(profile backend.Profile, name string, value int) {
	if a.profileFeatures == nil {
		a.profileFeatures = map[backend.Profile]map[string]int{}
	}
	values := a.profileFeatures[profile]
	if values == nil {
		values = map[string]int{}
		a.profileFeatures[profile] = values
	}
	values[name] = value
}

// rememberFeatures merges a set of values under one profile and writes
// the settings once.
func (a *App) rememberFeatures(profile backend.Profile, values map[string]int) {
	if profile == "" || len(values) == 0 {
		return
	}
	a.mu.Lock()
	for name, value := range values {
		a.rememberFeatureLocked(profile, name, value)
	}
	a.mu.Unlock()
	a.saveSettings()
}

// captureProfileFeatures records the values the machine is using now as
// the memory for p, including values the user never touched. That makes
// switching back restore the whole profile, not just the changed fields.
func (a *App) captureProfileFeatures(p backend.Profile, s backend.Snapshot) {
	a.mu.Lock()
	existing := a.profileFeatures[p]
	a.mu.Unlock()
	values := map[string]int{}
	for name, f := range s.Features {
		if !f.Supported || !rememberedFeature(name) {
			continue
		}
		if _, ok := existing[name]; ok {
			// The user's explicit value is newer than the snapshot.
			continue
		}
		values[name] = f.Value
	}
	a.rememberFeatures(p, values)
}

// featuresFor returns a copy of the remembered firmware values for p.
func (a *App) featuresFor(p backend.Profile) map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	values := a.profileFeatures[p]
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}

// rememberAura stores the current lighting scene for p.
func (a *App) rememberAura(p backend.Profile, preset auraPreset) {
	a.mu.Lock()
	if a.profileAura == nil {
		a.profileAura = map[backend.Profile]auraPreset{}
	}
	a.profileAura[p] = preset
	a.mu.Unlock()
	a.saveSettings()
}

// auraFor returns the saved lighting scene for p.
func (a *App) auraFor(p backend.Profile) (auraPreset, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	preset, ok := a.profileAura[p]
	return preset, ok
}
