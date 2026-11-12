// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// TestModelNameIsChinese checks that the Chinese window names a machine
// the way its market does, rather than repeating the DMI string.
func TestModelNameIsChinese(t *testing.T) {
	cases := []struct {
		product, want string
	}{
		{"ROG Zephyrus M16 GU603ZM", "ROG 幻 16 2022"},
		{"ROG Zephyrus M16 GU603ZW", "ROG 幻 16 2022"},
		{"ROG Zephyrus G14 GA401IV", "ROG 幻 14 2020"},
		{"ROG Zephyrus G16 GU605MV", "ROG 幻 16 Air 2024"},
		{"ROG Zephyrus M16 GU604VI", "ROG 幻 16 2023 星空版/经典版"},
		{"ROG Zephyrus M16", "ROG Zephyrus M16"},
	}
	for _, c := range cases {
		if got := modelName("zh-Hans", c.product); got != c.want {
			t.Errorf("modelName(zh-Hans, %q) = %q, want %q", c.product, got, c.want)
		}
	}
}

// TestModelNameStaysDMIInEnglish checks that other languages keep the
// name the firmware reports.
func TestModelNameStaysDMIInEnglish(t *testing.T) {
	product := "ROG Zephyrus M16 GU603ZM"
	if got := modelName("en", product); got != product {
		t.Errorf("modelName(en, %q) = %q, want the DMI name", product, got)
	}
}

// TestModelCode pinpoints the ASUS model code inside a DMI string.
func TestModelCode(t *testing.T) {
	cases := []struct{ product, want string }{
		{"ROG Zephyrus M16 GU603ZM", "GU603ZM"},
		{"GU603ZM", "GU603ZM"},
		{"ROG Ally RC71L", "RC71L"},
		{"ROG Zephyrus M16", ""},
		{"20", ""},
	}
	for _, c := range cases {
		if got := modelCode(c.product); got != c.want {
			t.Errorf("modelCode(%q) = %q, want %q", c.product, got, c.want)
		}
	}
}

// TestHeaderUsesChineseModelName checks the drawn window, not just the
// table: the header and overview subtitle say the Chinese name, and the
// English DMI string is gone.
func TestHeaderUsesChineseModelName(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("zh-Hans")
	tester.Frame()
	tester.Advance(600 * time.Millisecond)
	tester.Frame()

	if !tester.HasText("幻 16 2022") {
		t.Error("the Chinese window does not name the machine 幻 16 2022")
	}
	if tester.HasText("ROG Zephyrus M16 GU603ZM") {
		t.Error("the Chinese window still shows the English DMI name")
	}
}
