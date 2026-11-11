// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package backend

import "testing"

func TestProfileCodesRoundTrip(t *testing.T) {
	for _, p := range Profiles {
		if got := ProfileFromCode(p.Code()); got != p {
			t.Errorf("ProfileFromCode(%d) = %q, want %q", p.Code(), got, p)
		}
	}
	if got := ProfileFromCode(99); got != ProfileBalanced {
		t.Errorf("ProfileFromCode(99) = %q, want balanced", got)
	}
}

func TestCurvePoints(t *testing.T) {
	c := FanCurve{Temp: [8]uint8{30, 40, 50, 60, 70, 80, 90, 100}}
	for i := range c.PWM {
		c.PWM[i] = 255
	}
	for i, p := range c.Points() {
		if p[1] != 100 {
			t.Errorf("point %d is %.1f%%, want 100", i, p[1])
		}
		if p[0] != float64(c.Temp[i]) {
			t.Errorf("point %d is %.0f °C, want %d", i, p[0], c.Temp[i])
		}
	}
}

func TestCurveSetPointsClamps(t *testing.T) {
	var c FanCurve
	var pts [8][2]float64
	for i := range pts {
		pts[i] = [2]float64{-5, 130}
	}
	c.SetPoints(pts)
	for i := range c.Temp {
		if c.Temp[i] != 0 {
			t.Errorf("temperature %d = %d, want 0", i, c.Temp[i])
		}
		if c.PWM[i] != 255 {
			t.Errorf("PWM %d = %d, want 255", i, c.PWM[i])
		}
	}
}
