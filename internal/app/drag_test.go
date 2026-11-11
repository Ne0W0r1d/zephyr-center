// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend/fake"
)

// TestCurveDragWritesTheDraft drags the first CPU point and checks the
// app's draft keeps the edit across frames, which a copy of the frame's
// value would lose.
func TestCurveDragWritesTheDraft(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "fans"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	graph, ok := tester.Find("Fan curve")
	if !ok {
		t.Fatal("the curve editor drew no graph")
	}
	const key = "balanced/CPU"
	before := a.fanDraft[key][0]
	x, y := curveXY(float64(graph.X), float64(graph.Y), before[0], before[1], float64(graph.W), float64(graph.H))
	tester.Press(float32(x), float32(y))
	if _, held := a.fanDrag[key]; !held {
		t.Fatal("the press did not take a point")
	}
	tester.Move(float32(x+40), float32(y-20))
	tester.Release(float32(x+40), float32(y-20))
	tester.Frame()

	after := a.fanDraft[key][0]
	if after == before {
		t.Fatalf("the drag left the draft at %v", before)
	}
	if after[0] <= before[0] || after[1] <= before[1] {
		t.Errorf("the point went from %v to %v, want warmer and faster", before, after)
	}
	if _, held := a.fanDrag[key]; held {
		t.Error("the release left a point held")
	}
}

// TestCurveKeysMoveThePoint checks the arrow keys move the chosen point.
func TestCurveKeysMoveThePoint(t *testing.T) {
	a := New(fake.New())
	a.Refresh()
	a.page = "fans"
	tester := ui.NewTester(a.View, 1180, 800)
	tester.SetLocale("en")
	tester.Frame()

	graph, _ := tester.Find("Fan curve")
	const key = "balanced/CPU"
	before := a.fanDraft[key][0]
	x, y := curveXY(float64(graph.X), float64(graph.Y), before[0], before[1], float64(graph.W), float64(graph.H))
	tester.ClickAt(float32(x), float32(y))
	tester.Key(0, ui.KeyUp)
	tester.Frame()
	if after := a.fanDraft[key][0]; after[1] <= before[1] {
		t.Errorf("Up left the speed at %.0f, want more than %.0f", after[1], before[1])
	}
}
