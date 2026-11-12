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

// TestSetGPUDoesNotDeadlock picks a graphics mode and makes sure the click
// returns. setGPU once called Snapshot while holding the app's lock, which
// froze the window.
func TestSetGPUDoesNotDeadlock(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.Refresh()
	a.page = "gpu"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := tester.Click("Ultimate"); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("clicking a graphics mode blocked the view")
	}

	// The page marks the change as waiting for a restart rather than
	// pretending it is live.
	s := a.Snapshot()
	if s.GPU.Pending != backend.GPUModeUltimate || !s.GPU.NeedsReboot() {
		t.Errorf("after choosing Ultimate the state is %+v, want a pending restart", s.GPU)
	}
}

// TestGPURestartActionAppearsWithAPendingSwitch checks the restart button
// is offered only while the MUX change waits for POST.
func TestGPURestartActionAppearsWithAPendingSwitch(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "gpu"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	if tester.HasText("Restart now") {
		t.Fatal("the idle page offers a restart")
	}
	if err := tester.Click("Ultimate"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	if !a.Snapshot().GPU.NeedsReboot() {
		t.Fatalf("choosing Ultimate did not queue a restart: %+v", a.Snapshot().GPU)
	}
	if !tester.HasText("Restart now") {
		t.Fatalf("the pending page has no restart action: %q", tester.Texts())
	}
}

// TestSetGPUSwitchesToIntegratedLive checks that turning the discrete GPU
// off is a live change. Only the MUX, which Ultimate uses, needs a restart.
func TestSetGPUSwitchesToIntegratedLive(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "gpu"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	if err := tester.Click("Integrated"); err != nil {
		t.Fatal(err)
	}

	s := a.Snapshot()
	if s.GPU.Mode != backend.GPUModeIntegrated || s.GPU.Pending != "" || s.GPU.NeedsReboot() {
		t.Errorf("after choosing Integrated the state is %+v, want a live switch", s.GPU)
	}
}

// TestGPURestartsOnlyAroundUltimate pins when a graphics change needs a
// restart: entering or leaving the MUX path, never between Integrated and
// Hybrid.
func TestSetGPULeavesUltimateForARestart(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.Refresh()
	if err := be.SetGPUMode(context.Background(), backend.GPUModeUltimate); err != nil {
		t.Fatal(err)
	}
	a.Refresh()
	a.page = "gpu"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	if err := tester.Click("Integrated"); err != nil {
		t.Fatal(err)
	}

	// The MUX still points at the discrete GPU, so the switch cannot be
	// live: the page keeps Ultimate in use and queues Integrated.
	s := a.Snapshot()
	if s.GPU.Mode != backend.GPUModeUltimate {
		t.Errorf("mode is %q, want Ultimate until the restart", s.GPU.Mode)
	}
	if s.GPU.Pending != backend.GPUModeIntegrated || !s.GPU.NeedsReboot() {
		t.Errorf("the state is %+v, want Integrated queued for a restart", s.GPU)
	}
}

func TestGPURestartsOnlyAroundUltimate(t *testing.T) {
	cases := []struct {
		from, to backend.GPUMode
		want     bool
	}{
		{backend.GPUModeHybrid, backend.GPUModeIntegrated, false},
		{backend.GPUModeIntegrated, backend.GPUModeHybrid, false},
		{backend.GPUModeHybrid, backend.GPUModeUltimate, true},
		{backend.GPUModeIntegrated, backend.GPUModeUltimate, true},
		{backend.GPUModeUltimate, backend.GPUModeHybrid, true},
		{backend.GPUModeUltimate, backend.GPUModeIntegrated, true},
	}
	for _, c := range cases {
		if got := gpuRestarts(c.from, c.to); got != c.want {
			t.Errorf("gpuRestarts(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

// TestFirmwareListReadsAsSettings checks the firmware list names each
// attribute and says its value in words, not as a raw number with a list
// of possible values.
func TestFirmwareListReadsAsSettings(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "system"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	for _, want := range []string{
		"Startup sound",
		"Off",
		"Charging mode",
		"Standard charging",
		"NVIDIA dynamic boost",
		"15 W",
		"NVIDIA temperature target",
		"87 °C",
		"Panel overdrive",
		"Sustained CPU power (PL1)",
		"60 W",
		"Boost CPU power (PL2)",
		"135 W",
	} {
		if !tester.HasText(want) {
			t.Errorf("the firmware list has no %q", want)
		}
	}
	// The raw attribute names and the "value · [choices]" form are gone.
	for _, unwanted := range []string{"Boot Sound", "Charge Mode", "PPT PL1 Spl", "0 · [0 1]"} {
		if tester.HasText(unwanted) {
			t.Errorf("the firmware list still shows %q", unwanted)
		}
	}
}

// TestFirmwareListIsReadOnly checks the firmware list offers no controls:
// asusd marks several of these attributes read-only, and the list must not
// pretend otherwise.
func TestFirmwareListIsReadOnly(t *testing.T) {
	be := fake.New()
	a := New(be)
	a.Refresh()
	before, _ := be.Snapshot(context.Background())
	a.page = "system"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	// Clicking the charging mode's value, where a menu's trigger would be,
	// changes nothing.
	if err := tester.Click("Standard charging"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	a.Refresh()
	after, _ := be.Snapshot(context.Background())
	if got, want := after.Features["charge_mode"].Value, before.Features["charge_mode"].Value; got != want {
		t.Errorf("charge mode changed from %d to %d without a control", want, got)
	}
}
