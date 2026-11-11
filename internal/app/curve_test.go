// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package app

import "testing"

func TestCurveValueRoundTrip(t *testing.T) {
	const w, h = 800, 250
	for _, tc := range []struct{ temp, speed float64 }{{0, 0}, {50, 50}, {100, 100}, {35, 72}} {
		x, y := curveXY(0, 0, tc.temp, tc.speed, w, h)
		temp, speed := curveValue(x, y, w, h)
		if diff := temp - tc.temp; diff > 0.001 || diff < -0.001 {
			t.Errorf("temperature %.2f came back as %.2f", tc.temp, temp)
		}
		if diff := speed - tc.speed; diff > 0.001 || diff < -0.001 {
			t.Errorf("speed %.2f came back as %.2f", tc.speed, speed)
		}
	}
}

func TestMovePointKeepsTheCurveOrdered(t *testing.T) {
	pts := [8][2]float64{{30, 0}, {40, 20}, {50, 40}, {60, 60}, {70, 70}, {80, 80}, {90, 90}, {100, 100}}
	// Drag the third point far past its neighbours.
	movePoint(&pts, 2, 0, 0, 800, 250)
	if pts[2][0] != pts[1][0]+1 {
		t.Errorf("point 2's temperature is %.0f, want %.0f", pts[2][0], pts[1][0]+1)
	}
	movePoint(&pts, 2, 800, 0, 800, 250)
	if pts[2][0] != pts[3][0]-1 {
		t.Errorf("point 2's temperature is %.0f, want %.0f", pts[2][0], pts[3][0]-1)
	}
}

func TestNearestPoint(t *testing.T) {
	pts := [8][2]float64{{0, 0}, {10, 10}, {20, 20}, {30, 30}, {40, 40}, {50, 50}, {60, 60}, {70, 70}}
	x, y := curveXY(0, 0, pts[4][0], pts[4][1], 800, 250)
	if got := nearestPoint(pts, float32(x), float32(y), 800, 250); got != 4 {
		t.Errorf("nearest point is %d, want 4", got)
	}
	if got := nearestPoint(pts, 790, 10, 800, 250); got != -1 {
		t.Errorf("a far pointer chose point %d, want none", got)
	}
}
