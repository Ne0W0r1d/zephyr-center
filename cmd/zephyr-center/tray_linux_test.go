// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build linux

package main

import (
	"testing"

	"github.com/Ne0W0r1d/zephyr-center/internal/appicon"
)

func TestTrayPixmapsAreARGB32(t *testing.T) {
	pixmaps, err := trayPixmaps(appicon.TrayPNG)
	if err != nil {
		t.Fatal(err)
	}
	if len(pixmaps) != 4 {
		t.Fatalf("pixmaps = %d, want 4 sizes", len(pixmaps))
	}
	for _, p := range pixmaps {
		if p.Width != p.Height || p.Width < 16 {
			t.Errorf("pixmap is %dx%d, want a square icon", p.Width, p.Height)
		}
		if want := int(p.Width) * int(p.Height) * 4; len(p.Data) != want {
			t.Errorf("%dx%d data = %d bytes, want %d", p.Width, p.Height, len(p.Data), want)
		}
		visible := false
		for i := 0; i < len(p.Data); i += 4 {
			if p.Data[i] != 0 && (p.Data[i+1] != 0 || p.Data[i+2] != 0 || p.Data[i+3] != 0) {
				visible = true
				break
			}
		}
		if !visible {
			t.Errorf("%dx%d pixmap has no visible pixels", p.Width, p.Height)
		}
	}
}

func TestClampInt(t *testing.T) {
	cases := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{5, 10, 0, 10},
	}
	for _, tc := range cases {
		if got := clampInt(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}
