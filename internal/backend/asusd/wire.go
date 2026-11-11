// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

const (
	service   = "xyz.ljones.Asusd"
	rootPath  = "/xyz/ljones"
	platform  = "xyz.ljones.Platform"
	fanCurves = "xyz.ljones.FanCurves"
	auraIface = "xyz.ljones.Aura"
	armoury   = "xyz.ljones.AsusArmoury"
)

// auraModeWire is LedModeData as D-Bus delivers it: (uu(yyy)(yyy)ss).
//
// The two colors arrive as arrays of variants, not []byte, so they are
// kept as any and read by color3.
type auraModeWire struct {
	Mode      uint32
	Zone      uint32
	Color1    []any
	Color2    []any
	Speed     string
	Direction string
}

// rgbWire is one (yyy) tuple. godbus writes a [3]byte as "ay", so the
// channels must be three fields of a struct to get the tuple asusd's
// interface declares.
type rgbWire struct {
	R, G, B byte
}

// eightWire is one (yyyyyyyy) tuple, for the same reason.
type eightWire struct {
	A, B, C, D, E, F, G, H byte
}

// auraModeBodyWire is LedModeData as it is sent.
type auraModeBodyWire struct {
	Mode      uint32
	Zone      uint32
	Color1    rgbWire
	Color2    rgbWire
	Speed     string
	Direction string
}

// auraPowerWire is LedPower: (a(ubbbb)).
type auraPowerWire struct {
	Zones []struct {
		Zone     uint32
		Boot     bool
		Awake    bool
		Sleep    bool
		Shutdown bool
	}
}

// curveWire is FanCurveData's element as it arrives:
// (s(yyyyyyyy)(yyyyyyyy)b). godbus delivers the tuples as arrays of
// variants, so they are read by bytes8.
type curveWire struct {
	Fan     string
	PWM     []any
	Temp    []any
	Enabled bool
}

// bytes8 reads eight channel values from a D-Bus tuple.
func bytes8(v []any) ([8]byte, bool) {
	var out [8]byte
	if len(v) != 8 {
		return out, false
	}
	for i, x := range v {
		switch n := x.(type) {
		case byte:
			out[i] = n
		case int32:
			out[i] = byte(n)
		case uint32:
			out[i] = byte(n)
		case int64:
			out[i] = byte(n)
		case uint64:
			out[i] = byte(n)
		default:
			return out, false
		}
	}
	return out, true
}

// curveBodyWire is the same element as it is sent.
type curveBodyWire struct {
	Fan     string
	PWM     eightWire
	Temp    eightWire
	Enabled bool
}

// convertMode turns the wire form into the backend's value, defaulting
// missing colors to black.
func convertMode(w auraModeWire) backend.AuraMode {
	m := backend.AuraMode{
		Mode:      w.Mode,
		Zone:      w.Zone,
		Speed:     w.Speed,
		Direction: w.Direction,
	}
	m.Color1 = color3(w.Color1)
	m.Color2 = color3(w.Color2)
	return m
}

// color3 reads three channel values from a D-Bus array.
func color3(v []any) [3]byte {
	var out [3]byte
	for i := 0; i < 3 && i < len(v); i++ {
		switch n := v[i].(type) {
		case byte:
			out[i] = n
		case int32:
			out[i] = byte(n)
		case uint32:
			out[i] = byte(n)
		case int64:
			out[i] = byte(n)
		case uint64:
			out[i] = byte(n)
		}
	}
	return out
}

// convertCurve turns the wire form into the backend's curve, padding
// arrays D-Bus may have shortened.
func convertCurve(w curveWire) (backend.FanCurve, error) {
	pwm, ok := bytes8(w.PWM)
	if !ok {
		return backend.FanCurve{}, fmt.Errorf("asusd: fan curve %q has %d PWM points, want 8", w.Fan, len(w.PWM))
	}
	temp, ok := bytes8(w.Temp)
	if !ok {
		return backend.FanCurve{}, fmt.Errorf("asusd: fan curve %q has %d temperature points, want 8", w.Fan, len(w.Temp))
	}
	return backend.FanCurve{Fan: w.Fan, PWM: pwm, Temp: temp, Enabled: w.Enabled}, nil
}

// curveBody builds the D-Bus body for SetFanCurve.
func curveBody(c backend.FanCurve) any {
	return curveBodyWire{Fan: c.Fan, PWM: eightOf(c.PWM), Temp: eightOf(c.Temp), Enabled: c.Enabled}
}

// eightOf turns a byte array into the struct D-Bus sends as a tuple.
func eightOf(v [8]byte) eightWire {
	return eightWire{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]}
}

// auraModeBody builds the D-Bus body for SetLedModeData.
func auraModeBody(m backend.AuraMode) any {
	return auraModeBodyWire{
		Mode:      m.Mode,
		Zone:      m.Zone,
		Color1:    rgbWire{m.Color1[0], m.Color1[1], m.Color1[2]},
		Color2:    rgbWire{m.Color2[0], m.Color2[1], m.Color2[2]},
		Speed:     m.Speed,
		Direction: m.Direction,
	}
}

// auraPowerBody builds the D-Bus body for SetLedPower.
func auraPowerBody(p backend.AuraPower) any {
	var w auraPowerWire
	w.Zones = append(w.Zones, struct {
		Zone     uint32
		Boot     bool
		Awake    bool
		Sleep    bool
		Shutdown bool
	}{p.Zone, p.Boot, p.Awake, p.Sleep, p.Shutdown})
	return w
}

// variantBytes returns a property's bytes, or nil when it is not an
// array of bytes.
func variantBytes(v dbus.Variant) []byte {
	b, _ := v.Value().([]byte)
	return b
}
