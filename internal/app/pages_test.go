// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
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
