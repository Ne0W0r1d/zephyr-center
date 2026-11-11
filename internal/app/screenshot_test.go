// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// TestScreenshots writes a PNG of every page, light and dark, when
// ZEPHYR_SCREENSHOTS names a directory: the same test doubles as a gallery
// of the MVP.
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("ZEPHYR_SCREENSHOTS")
	if dir == "" {
		t.Skip("set ZEPHYR_SCREENSHOTS to a directory to write the screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pages := []string{"overview", "performance", "fans", "aura", "gpu", "system", "about"}
	for _, dark := range []bool{false, true} {
		for _, page := range pages {
			a := New(fake.New())
			a.Refresh()
			a.page = page
			a.fanPage = "balanced"
			tester := ui.NewTester(a.View, 1180, 800)
			tester.SetDark(dark)
			tester.SetLocale("en")
			tester.Frame()
			// CrychicUI's entrance animations start transparent; let them
			// finish so the screenshot shows the settled page.
			tester.Advance(600 * time.Millisecond)
			tester.Frame()
			name := filepath.Join(dir, "crychic-"+page+"-"+appearance(dark)+".png")
			f, err := os.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tester.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

func appearance(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}
