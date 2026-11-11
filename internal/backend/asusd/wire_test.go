// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

func TestAuraModeBodyHasTheFirmwareSignature(t *testing.T) {
	m := backend.AuraMode{Mode: 1, Zone: 2, Color1: [3]byte{1, 2, 3}, Color2: [3]byte{4, 5, 6}, Speed: "Fast", Direction: "Left"}
	if got, want := dbus.MakeVariant(auraModeBody(m)).Signature().String(), "(uu(yyy)(yyy)ss)"; got != want {
		t.Errorf("signature %q, want %q", got, want)
	}
}

func TestAuraModeBodyRoundTrips(t *testing.T) {
	in := backend.AuraMode{Mode: 3, Zone: 1, Color1: [3]byte{227, 94, 242}, Color2: [3]byte{0, 120, 255}, Speed: "Slow", Direction: "Up"}
	v := dbus.MakeVariant(auraModeBody(in))
	// The body's fixed tuples read back as arrays of variants.
	var raw auraModeBodyWire
	if err := dbus.Store([]any{v}, &raw); err != nil {
		t.Fatal(err)
	}
	out := backend.AuraMode{
		Mode: raw.Mode, Zone: raw.Zone,
		Color1:    [3]byte{raw.Color1.R, raw.Color1.G, raw.Color1.B},
		Color2:    [3]byte{raw.Color2.R, raw.Color2.G, raw.Color2.B},
		Speed:     raw.Speed,
		Direction: raw.Direction,
	}
	if out != in {
		t.Errorf("round trip gave %+v, want %+v", out, in)
	}
}

func TestCurveBodyHasTheFirmwareSignature(t *testing.T) {
	c := backend.FanCurve{Fan: "CPU"}
	if got, want := dbus.MakeVariant(curveBody(c)).Signature().String(), "(s(yyyyyyyy)(yyyyyyyy)b)"; got != want {
		t.Errorf("signature %q, want %q", got, want)
	}
}

func TestConvertCurveRejectsShortCurves(t *testing.T) {
	if _, err := convertCurve(curveWire{Fan: "CPU", PWM: []any{1}, Temp: []any{2}}); err == nil {
		t.Fatal("a one-point curve was accepted")
	}
}

func TestColor3(t *testing.T) {
	if got := color3([]any{uint32(1), int32(2), byte(3)}); got != [3]byte{1, 2, 3} {
		t.Errorf("color3 = %v, want [1 2 3]", got)
	}
	if got := color3(nil); got != [3]byte{} {
		t.Errorf("color3(nil) = %v, want zero", got)
	}
}

func TestPPDProfilesReadsTheDaemonDict(t *testing.T) {
	v := dbus.MakeVariant([]map[string]dbus.Variant{
		{"Profile": dbus.MakeVariant("power-saver")},
		{"Profile": dbus.MakeVariant("balanced")},
		{"Profile": dbus.MakeVariant("balanced")},
		{"Profile": dbus.MakeVariant("performance")},
	})
	got := ppdProfiles(v)
	want := []backend.PPDPreset{backend.PPDPowerSaver, backend.PPDBalanced, backend.PPDPerformance}
	if len(got) != len(want) {
		t.Fatalf("ppdProfiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("preset %d = %q, want %q", i, got[i], want[i])
		}
	}
}
