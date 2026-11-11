// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// fakeWindow records what the client-side title bar asked it to do.
type fakeWindow struct {
	minimized   bool
	toggled     bool
	closed      bool
	isMaximized bool
}

func (w *fakeWindow) Minimize()       { w.minimized = true }
func (w *fakeWindow) ToggleMaximize() { w.toggled = true }
func (w *fakeWindow) Close()          { w.closed = true }
func (w *fakeWindow) IsMaximized() bool {
	return w.isMaximized
}

// TestWindowControlsDrawClientSideChrome checks that every platform gets
// the same three controls, and that they reach the window.
func TestWindowControlsDrawClientSideChrome(t *testing.T) {
	w := &fakeWindow{}
	a := New(fake.New())
	a.AttachWindow(w)
	a.Refresh()
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	for _, label := range []string{"Minimize", "Maximize", "Close"} {
		if !tester.HasText(label) {
			t.Errorf("the title bar has no %s control", label)
		}
	}

	if err := tester.Click("Minimize"); err != nil {
		t.Fatal(err)
	}
	if !w.minimized {
		t.Error("Minimize did not reach the window")
	}
	if err := tester.Click("Maximize"); err != nil {
		t.Fatal(err)
	}
	if !w.toggled {
		t.Error("Maximize did not reach the window")
	}
	if err := tester.Click("Close"); err != nil {
		t.Fatal(err)
	}
	if !w.closed {
		t.Error("Close did not reach the window")
	}
}

// TestWindowControlsShowRestore checks that a maximized window offers
// Restore instead of a second maximize action.
func TestWindowControlsShowRestore(t *testing.T) {
	a := New(fake.New())
	a.AttachWindow(&fakeWindow{isMaximized: true})
	a.Refresh()
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	if !tester.HasText("Restore") {
		t.Fatal("a maximized window has no Restore control")
	}
	if tester.HasText("Maximize") {
		t.Fatal("a maximized window still shows Maximize")
	}
}
