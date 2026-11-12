// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// TestProfileSwitchRestoresPowerValues checks that PL and NVIDIA limits
// follow their profile instead of leaking into the next one.
func TestProfileSwitchRestoresPowerValues(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.Refresh()

	a.setFeature("ppt_pl1_spl", 55)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Features["ppt_pl1_spl"].Value == 55
	})

	a.setProfile(backend.ProfilePerformance)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Profile == backend.ProfilePerformance && s.Features["ppt_pl1_spl"].Value == 55
	})
	a.setFeature("ppt_pl1_spl", 80)
	a.setFeature("nv_dynamic_boost", 20)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Features["ppt_pl1_spl"].Value == 80 && s.Features["nv_dynamic_boost"].Value == 20
	})

	a.setProfile(backend.ProfileBalanced)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Profile == backend.ProfileBalanced &&
			s.Features["ppt_pl1_spl"].Value == 55 &&
			s.Features["nv_dynamic_boost"].Value == 15
	})

	a.setProfile(backend.ProfilePerformance)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Profile == backend.ProfilePerformance &&
			s.Features["ppt_pl1_spl"].Value == 80 &&
			s.Features["nv_dynamic_boost"].Value == 20
	})
}

// TestPowerModeIsNotGenericProfileMemory keeps Windows power plans on the
// explicit binding card only, so an unbound profile never restores a plan
// the user did not ask for.
func TestPowerModeIsNotGenericProfileMemory(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.Refresh()

	a.setFeature("power_mode", 3)
	if _, ok := a.featuresFor(backend.ProfileBalanced)["power_mode"]; ok {
		t.Error("power_mode was stored as a generic profile feature")
	}
}

// TestAuraEffectOptionsIncludesZoneModes checks zone-specific effects are
// offered even when the daemon omits them from SupportedBasicModes.
func TestAuraEffectOptionsIncludesZoneModes(t *testing.T) {
	options := auraEffectOptions(backend.AuraState{
		Modes:    []uint32{0, 1},
		AllModes: []backend.AuraMode{{Mode: 11}, {Mode: 1}, {Mode: 9}},
	})
	if len(options) != 3 || options[0] != 0 || options[1] != 1 || options[2] != 11 {
		t.Errorf("effect options are %v, want [0 1 11]", options)
	}
}

// TestProfileScenesAndBindingsPersist checks the settings file keeps the
// Windows power binding, firmware values and lighting scene.
func TestProfileScenesAndBindingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsFileName)
	a := New(fake.New())
	a.settingsPath = path
	a.rememberFeature(backend.ProfileQuiet, "nv_temp_target", 80)
	a.setPowerModeBinding(backend.ProfileQuiet, 0)
	a.rememberAura(backend.ProfileQuiet, auraPreset{
		Mode: 2, Zone: 1, Color1: [3]byte{1, 2, 3}, Color2: [3]byte{4, 5, 6},
		Speed: "Slow", Direction: "Left", Brightness: 1,
	})

	b := New(fake.New())
	b.settingsPath = path
	b.loadSettings()
	if got := b.featuresFor(backend.ProfileQuiet)["nv_temp_target"]; got != 80 {
		t.Errorf("saved nv_temp_target is %d, want 80", got)
	}
	if got, ok := b.powerModeBindingFor(backend.ProfileQuiet); !ok || got != 0 {
		t.Errorf("saved power mode is %d, %v, want 0, true", got, ok)
	}
	scene, ok := b.auraFor(backend.ProfileQuiet)
	if !ok || scene.Mode != 2 || scene.Brightness != 1 || scene.Color2 != [3]byte{4, 5, 6} {
		t.Errorf("saved lighting scene is %+v, %v", scene, ok)
	}
}

// TestProfileSwitchRestoresLightingScene checks a scene follows the
// profile, including brightness.
func TestProfileSwitchRestoresLightingScene(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.Refresh()

	a.auraMode, a.auraZone = 2, 0
	a.auraSpeed, a.auraDirection = "Fast", "Left"
	a.auraColor1, a.auraColor2 = ui.RGB(10, 20, 30), ui.RGB(40, 50, 60)
	a.auraBrightness = 3
	a.auraDirty, a.auraSynced = true, true
	a.applyAura()

	a.setProfile(backend.ProfilePerformance)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Profile == backend.ProfilePerformance &&
			s.Aura.Mode.Mode == 2 && s.Aura.Brightness == 3
	})
	a.auraMode = 5
	a.auraBrightness = 1
	a.auraDirty = true
	a.applyAura()
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Aura.Mode.Mode == 5 && s.Aura.Brightness == 1
	})

	a.setProfile(backend.ProfileBalanced)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Profile == backend.ProfileBalanced &&
			s.Aura.Mode.Mode == 2 && s.Aura.Brightness == 3
	})
}

// TestPowerBindingCardShowsOnWindowsModel checks the PPD card becomes the
// Windows power-plan binding when power-profiles-daemon is absent.
func TestPowerBindingCardShowsOnWindowsModel(t *testing.T) {
	be := fake.New()
	s, _ := be.Snapshot(context.Background())
	s.PPD.Supported = false
	a := New(be)
	a.snap.Store(&s)
	a.page = "performance"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	if !tester.HasText("Windows power plan binding") {
		t.Fatalf("performance page has no Windows binding card: %q", tester.Texts())
	}
}

// TestStartupRestoresExplicitSettings checks a launch reapplies only the
// values the user saved, without touching an untouched PPD default.
func TestStartupRestoresExplicitSettings(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.rememberFeature(backend.ProfileBalanced, "ppt_pl1_spl", 55)
	a.setPowerModeBinding(backend.ProfileBalanced, 2)
	a.rememberAura(backend.ProfileBalanced, auraPreset{
		Mode: 2, Zone: 0, Speed: "Fast", Direction: "Left",
		Color1: [3]byte{1, 2, 3}, Color2: [3]byte{4, 5, 6}, Brightness: 1,
	})
	a.Refresh()
	a.applyStartupSettings()

	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Features["ppt_pl1_spl"].Value == 55 &&
			s.Features["power_mode"].Value == 2 &&
			s.Aura.Mode.Mode == 2 && s.Aura.Brightness == 1
	})
	if s, _ := be.Snapshot(context.Background()); s.PPD.Active == backend.PPDPowerSaver {
		t.Error("startup changed an untouched PPD binding")
	}
}
