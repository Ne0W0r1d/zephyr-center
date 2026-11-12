// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
	"github.com/Ne0W0r1d/zephyr-center/internal/i18n"
)

// TestPagesRender draws every page in English and Chinese, so a missing
// key or a panicking widget fails the test.
func TestPagesRender(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		for _, page := range pageNames {
			a := New(fake.New())
			a.Refresh()
			a.page = page
			tester := ui.NewTester(a.View, 1180, 800)
			tester.SetLocale(lang)
			tester.Frame()
			if len(tester.Texts()) == 0 {
				t.Errorf("%s %s drew no text", lang, page)
			}
		}
	}
}

// TestNavigation switches pages by clicking the sidebar.
func TestNavigation(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "performance"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	if !tester.HasText("Overview") {
		t.Fatal("the window did not open on the overview")
	}
	if err := tester.Click("Fans"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !tester.HasText("Fan curves for each profile") {
		t.Fatalf("clicking Fans showed %q", tester.Texts())
	}
	if a.page != "fans" {
		t.Errorf("the page is %q, want fans", a.page)
	}
}

// TestProfileChoiceApplies checks that a profile row writes to the backend.
func TestProfileChoiceApplies(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "performance"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	// The description is unique to the page's radio row; "Performance" also
	// names the sidebar item.
	if err := tester.Click("The fans spin faster and the limits rise."); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	a.Refresh()
	if got := a.Snapshot().Profile; got != "performance" {
		t.Errorf("profile is %q, want performance", got)
	}
}

// TestTurboProfileName checks the name of the firmware's turbo profile:
// Windows calls it Enhanced, so it does not read as the Windows power-mode
// slider on the same page.
func TestTurboProfileName(t *testing.T) {
	want := map[string]string{"en": "Performance", "zh-Hans": "性能"}
	if runtime.GOOS == "windows" {
		want = map[string]string{"en": "Enhanced", "zh-Hans": "增强"}
	}
	for lang, name := range want {
		a := New(fake.New())
		a.Refresh()
		a.page = "performance"
		tester := ui.NewTester(a.View, 1180, 800)
		tester.SetLocale(lang)
		tester.Frame()
		if !tester.HasText(name) {
			t.Errorf("%s calls the turbo profile something other than %q", lang, name)
		}
	}
}

// TestBatteryCareLivesOnOverview checks that the charge limit moved out of
// the performance page and into the battery card.
func TestBatteryCareLivesOnOverview(t *testing.T) {
	a := New(fake.New())
	a.settingsPath = ""
	a.Refresh()
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	if !tester.HasText("Battery care") {
		t.Fatal("the overview has no battery care control")
	}

	a.page = "performance"
	tester.Frame()
	if tester.HasText("Battery care") {
		t.Error("the performance page still shows battery care")
	}
}

// TestChargeToFullOnce checks that the one-time full charge lifts the
// limit and puts it back when the battery is full.
func TestChargeToFullOnce(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.Refresh()

	a.chargeToFull(80)
	if got := a.chargeOnceLimit(); got != 80 {
		t.Fatalf("chargeOnce is %d, want 80", got)
	}
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.ChargeLimit == 100
	})

	s := backend.Snapshot{Battery: backend.Battery{Present: true, Capacity: 100}}
	a.restoreChargeOnce(&s)
	if s.ChargeLimit != 80 {
		t.Errorf("snapshot limit is %d, want 80", s.ChargeLimit)
	}
	if got := a.chargeOnceLimit(); got != 0 {
		t.Errorf("chargeOnce is %d after restoring, want 0", got)
	}
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.ChargeLimit == 80
	})
}

// TestProfileAppliesPPDBinding checks that choosing a firmware mode also
// switches power-profiles-daemon to the preset bound to it.
func TestProfileAppliesPPDBinding(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.mu.Lock()
	a.ppdBindings = defaultPPDBindings()
	a.mu.Unlock()
	a.Refresh()

	a.setProfile(backend.ProfileQuiet)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.PPD.Active == backend.PPDPowerSaver
	})
}

// TestBindingChangeAppliesToCurrentProfile checks that rebinding the mode
// in use switches PPD at once.
func TestBindingChangeAppliesToCurrentProfile(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.Refresh()

	a.setPPDBinding(backend.ProfileBalanced, backend.PPDPerformance)
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.PPD.Active == backend.PPDPerformance
	})
}

// TestPowerLimitsSurviveBadValues draws the performance page over the
// readings a firmware that does not implement the PPT endpoints returns:
// a zero beside a real range. The stepper once panicked on them, taking
// the whole window down.
func TestPowerLimitsSurviveBadValues(t *testing.T) {
	for _, value := range []int{0, -65536, 200} {
		be := fake.New()
		s, _ := be.Snapshot(context.Background())
		for _, name := range []string{"ppt_pl1_spl", "ppt_pl2_sppt", "nv_dynamic_boost", "nv_temp_target"} {
			f := s.Features[name]
			f.Value = value
			s.Features[name] = f
		}
		a := New(be)
		a.snap.Store(&s)
		a.page = "performance"
		tester := ui.NewTester(a.View, 1180, 800)
		tester.SetLocale("en")
		tester.Frame()
		if !tester.HasText("Power limits") {
			t.Errorf("value %d: the performance page drew no power limits", value)
		}
	}
}

// TestUnsupportedFeaturesStayHidden checks the firmware report lists only
// what the firmware answers for: an endpoint that reads back nothing must
// not show as a plausible 0 W.
func TestUnsupportedFeaturesStayHidden(t *testing.T) {
	be := fake.New()
	s, _ := be.Snapshot(context.Background())
	f := s.Features["nv_dynamic_boost"]
	f.Supported = false
	f.Value = 0
	s.Features["nv_dynamic_boost"] = f
	a := New(be)
	a.snap.Store(&s)
	a.page = "system"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	if tester.HasText("NVIDIA dynamic boost") {
		t.Error("the firmware report lists an unsupported attribute")
	}
	if !tester.HasText("Sustained CPU power (PL1)") {
		t.Error("the firmware report dropped a supported attribute")
	}
}

// TestPowerStepperWritesTheLimit checks the plus button of a power limit
// writes the next value to the backend.
func TestPowerStepperWritesTheLimit(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.Refresh()
	a.page = "performance"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	label := i18n.TLocale("en", "performance.ppt_pl1")
	increase := fmt.Sprintf("Increase %s", label)
	box, ok := tester.Find(increase)
	if !ok {
		t.Fatalf("the performance page drew no %q", increase)
	}
	if box.Y+box.H > 800 {
		tester.Scroll(590, 400, 0, box.Y+box.H-760)
		tester.Frame()
	}
	if err := tester.Click(increase); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Features["ppt_pl1_spl"].Value == 61
	})
}

// TestAdvancedPowerFolds checks the advanced power controls stay folded
// until the pane is opened, and that a Windows power choice writes through.
func TestAdvancedPowerFolds(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.settingsPath = ""
	a.Refresh()
	a.page = "performance"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	if tester.HasText("Processor performance boost") {
		t.Fatal("the advanced controls show before the pane is opened")
	}
	scrollIntoView(t, tester, "Advanced")
	if err := tester.Click("Advanced"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(600 * time.Millisecond)
	tester.Frame()
	if !a.advancedOpen {
		t.Fatal("clicking Advanced did not open the pane")
	}
	if !tester.HasText("Processor performance boost") || !tester.HasText("Windows power mode") {
		t.Fatalf("the opened pane shows %q", tester.Texts())
	}

	// The fake machine boosts aggressively (2); choosing Disabled writes 0.
	scrollIntoView(t, tester, "Aggressive")
	if err := tester.Click("Aggressive"); err != nil {
		t.Fatal(err)
	}
	tester.Advance(300 * time.Millisecond)
	tester.Frame()
	scrollIntoView(t, tester, "Disabled")
	if err := tester.Click("Disabled"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		s, _ := be.Snapshot(context.Background())
		return s.Features["cpu_boost"].Value == 0
	})
}

// scrollIntoView scrolls the page until the named element is inside the
// 800-DIP-tall test window.
func scrollIntoView(t *testing.T, tester *ui.Tester, name string) {
	t.Helper()
	for i := 0; i < 12; i++ {
		box, ok := tester.Find(name)
		if !ok {
			t.Fatalf("the page drew no %q", name)
		}
		if box.Y >= 0 && box.Y+box.H <= 790 {
			return
		}
		tester.Scroll(590, 400, 0, box.Y+box.H-700)
		tester.Frame()
	}
	t.Fatalf("%q did not scroll into view", name)
}

// waitFor polls cond until it holds or the test times out.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not hold in time")
}
