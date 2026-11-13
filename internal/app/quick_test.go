// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

func TestQuickPanelShowsReadingsAndSwitches(t *testing.T) {
	a := New(fake.New())
	a.settingsPath = ""
	a.Refresh()

	opened, closed, quit := 0, 0, 0
	tester := ui.NewTester(a.QuickView(
		func() { opened++ },
		func() { closed++ },
		func() { quit++ },
	), 400, 560)
	tester.SetLocale("en")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	for _, text := range []string{
		"CPU", "GPU", "61 °C", "47 °C", "3400 RPM", "3100 RPM",
		"Hybrid", "Quiet", "Balanced", "Performance",
		"Integrated", "Ultimate", "Quit",
	} {
		if !tester.HasText(text) {
			t.Errorf("quick panel has no %q: %q", text, tester.Texts())
		}
	}

	if err := tester.Click("Performance"); err != nil {
		t.Fatal(err)
	}
	if got := a.Snapshot().Profile; got != backend.ProfilePerformance {
		t.Fatalf("profile = %q, want performance", got)
	}

	// The performance click rebuilds the panel on the next frame. Ultimate
	// is offered by the fresh snapshot rather than the old text tree.
	tester.Frame()
	if err := tester.Click("Ultimate"); err != nil {
		t.Fatal(err)
	}
	if gpu := a.Snapshot().GPU; gpu.Pending != backend.GPUModeUltimate || !gpu.NeedsReboot() {
		t.Fatalf("graphics overlay = %+v, want Ultimate queued for restart", gpu)
	}

	if err := tester.Click("Quit"); err != nil {
		t.Fatal(err)
	}
	if quit != 1 {
		t.Errorf("quit callback ran %d times, want 1", quit)
	}

	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if closed != 1 {
		t.Errorf("Escape closed the panel %d times, want 1", closed)
	}
	if opened != 0 {
		t.Errorf("open callback ran %d times, want 0", opened)
	}
}

func TestQuickPanelScreenshotWhenAsked(t *testing.T) {
	path := os.Getenv("ZEPHYR_QUICK_SCREENSHOT")
	if path == "" {
		t.Skip("set ZEPHYR_QUICK_SCREENSHOT to a PNG path to write the quick panel")
	}
	a := New(fake.New())
	a.settingsPath = ""
	a.Refresh()
	tester := ui.NewTester(a.QuickView(func() {}, func() {}, func() {}), 400, 560)
	tester.SetLocale("zh-Hans")
	tester.SetDark(true)
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tester.Image()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
