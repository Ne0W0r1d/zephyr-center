// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

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
