// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package ghelper

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// fakeBridge is a shell script that answers the protocol, standing in for
// the Windows helper on this machine.
const fakeBridge = `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"snapshot"'*) echo '{"ok":true,"backend":"ghelper","version":"0.1.0","snapshot":{"connected":true,"model":"ROG Zephyrus G14","board":"GA402","bios":"GA402.312","bios_date":"2023-01-18","cpu":"AMD Ryzen 9","igpu":"AMD Radeon 680M","dgpu":"NVIDIA GeForce RTX 4060","kernel":"22631","os":"Microsoft Windows 11 Pro","profile":"performance","profile_choices":["quiet","balanced","performance"],"charge_limit":80,"charge_supported":true,"sensors":{"cpu_temp":62,"gpu_temp":51,"cpu_rpm":3000,"gpu_rpm":2800},"battery":{"present":true,"capacity":71,"status":"Charging","power_w":42.5,"health":88.2,"cycles":120,"time_left":"2h 10m"},"gpu":{"supported":true,"mode":"hybrid","modes":["integrated","hybrid","ultimate"],"pending":"ultimate","reboot":true},"aura":{"supported":true,"modes":[0,1,2,3],"brightness":2,"brightnesses":[0,1,2,3],"mode":{"mode":1,"zone":0,"color1":[207,56,93],"color2":[12,143,89],"speed":"Fast","direction":"Left"},"power":[{"zone":1,"boot":true,"awake":true,"sleep":false,"shutdown":true}],"power_zones":[1],"zones":[0],"device_type":0,"path":"hid"},"features":{"panel_overdrive":{"supported":true,"value":1,"values":[0,1]}},"curves":{"balanced":[{"fan":"CPU","temp":[30,40,50,60,70,80,90,100],"pwm":[0,64,128,160,192,224,240,255],"enabled":true}]}}}' ;;
    *) echo '{"ok":true}' ;;
  esac
done
`

// newFake returns a backend that runs the script.
func newFake(t *testing.T) *Backend {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("needs a POSIX shell")
	}
	path := t.TempDir() + "/bridge.sh"
	if err := os.WriteFile(path, []byte(fakeBridge), 0o755); err != nil {
		t.Fatal(err)
	}
	return New(path)
}

func TestSnapshotOverTheProtocol(t *testing.T) {
	b := newFake(t)
	s, err := b.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Err != "" {
		t.Fatalf("snapshot error: %s", s.Err)
	}
	if s.Backend != "ghelper" || s.Info.Product != "ROG Zephyrus G14" {
		t.Errorf("snapshot is %+v", s)
	}
	if s.Profile != backend.ProfilePerformance {
		t.Errorf("profile is %q", s.Profile)
	}
	if s.GPU.Mode != backend.GPUModeHybrid || len(s.GPU.Modes) != 3 {
		t.Errorf("GPU is %+v", s.GPU)
	}
	if s.GPU.Pending != backend.GPUModeUltimate || !s.GPU.NeedsReboot() {
		t.Errorf("GPU pending is %+v", s.GPU)
	}
	curves := s.Curves[backend.ProfileBalanced]
	if len(curves) != 1 || curves[0].PWM[7] != 255 || curves[0].Temp[0] != 30 || !curves[0].Enabled {
		t.Errorf("curves are %+v", curves)
	}
	if f := s.Features["panel_overdrive"]; !f.Supported || f.Value != 1 {
		t.Errorf("feature is %+v", f)
	}
	if s.Info.Board != "GA402" || s.Info.BIOSDate != "2023-01-18" || s.Info.DGPU == "" || s.Info.OS == "" {
		t.Errorf("system info is %+v", s.Info)
	}
	if s.Battery.Health != 88.2 || s.Battery.Cycles != 120 || s.Battery.TimeLeft != "2h 10m" || s.Battery.PowerW != 42.5 {
		t.Errorf("battery is %+v", s.Battery)
	}
	if !s.Aura.Supported || s.Aura.Brightness != 2 || s.Aura.Mode.Mode != 1 ||
		s.Aura.Mode.Color1 != [3]byte{207, 56, 93} || s.Aura.Mode.Speed != "Fast" ||
		len(s.Aura.Power) != 1 || s.Aura.Power[0].Sleep {
		t.Errorf("aura is %+v", s.Aura)
	}
}

func TestRequestEncoding(t *testing.T) {
	b := newFake(t)
	if err := b.SetProfile(context.Background(), backend.ProfileQuiet); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFeature(context.Background(), "ppt_pl1_spl", 45); err != nil {
		t.Fatal(err)
	}
	if err := b.SetGPUMode(context.Background(), backend.GPUModeUltimate); err != nil {
		t.Fatal(err)
	}
}

// TestWindowsParityWrites checks that lighting and per-profile fan
// controls reach the bridge instead of being reported as unsupported.
func TestWindowsParityWrites(t *testing.T) {
	logPath := t.TempDir() + "/requests.log"
	t.Setenv("BRIDGE_LOG", logPath)
	script := "#!/bin/sh\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$BRIDGE_LOG\"; echo '{\"ok\":true}'; done\n"
	path := t.TempDir() + "/record.sh"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b := New(path)
	ctx := context.Background()

	curve := backend.FanCurve{Fan: "GPU", Enabled: true}
	for i := range curve.Temp {
		curve.Temp[i] = uint8(30 + i*10)
		curve.PWM[i] = uint8(i * 36)
	}
	if err := b.SetFanCurve(ctx, backend.ProfileQuiet, curve); err != nil {
		t.Fatal(err)
	}
	if err := b.SetFanCurvesEnabled(ctx, backend.ProfileQuiet, true); err != nil {
		t.Fatal(err)
	}
	if err := b.ResetFanCurves(ctx, backend.ProfileQuiet); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAura(ctx, backend.AuraMode{
		Mode: 3, Zone: 0, Color1: [3]byte{1, 2, 3}, Color2: [3]byte{4, 5, 6},
		Speed: "Fast", Direction: "Left",
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAuraBrightness(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.SetAuraPower(ctx, backend.AuraPower{Zone: 1, Boot: true, Awake: true}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 6 {
		t.Fatalf("the bridge saw %d requests, want 6", len(lines))
	}
	requests := make([]request, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal([]byte(line), &requests[i]); err != nil {
			t.Fatalf("request %d is %q: %v", i, line, err)
		}
	}
	if requests[0].Op != "set_fan_curve" || requests[0].Profile != "quiet" ||
		requests[0].Fan != "GPU" || !requests[0].Enabled {
		t.Errorf("fan curve request is %+v", requests[0])
	}
	if requests[1].Op != "set_fan_curves_enabled" || !requests[1].Enabled {
		t.Errorf("fan enable request is %+v", requests[1])
	}
	if requests[2].Op != "reset_fan_curves" {
		t.Errorf("fan reset request is %+v", requests[2])
	}
	if requests[3].Op != "set_aura" || requests[3].Mode != 3 ||
		len(requests[3].Color1) != 3 || requests[3].Color1[2] != 3 {
		t.Errorf("aura request is %+v", requests[3])
	}
	if requests[4].Op != "set_aura_brightness" {
		t.Errorf("brightness request is %+v", requests[4])
	}
	if requests[5].Op != "set_aura_power" || !requests[5].Boot {
		t.Errorf("power request is %+v", requests[5])
	}
}

// TestBridgeFailureIsReported checks a non-OK reply becomes an error.
func TestBridgeFailureIsReported(t *testing.T) {
	path := t.TempDir() + "/fail.sh"
	script := "#!/bin/sh\nread line\necho '{\"ok\":false,\"error\":\"the firmware refused the write\"}'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b := New(path)
	err := b.SetProfile(context.Background(), backend.ProfileQuiet)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("error is %v, want the bridge's message", err)
	}
}

// TestConvertKeepsTheWireValues checks the JSON field names, which the C#
// side writes in snake_case.
func TestConvertKeepsTheWireValues(t *testing.T) {
	body := `{"ok":true,"snapshot":{"model":"X","profile":"quiet","profile_choices":["quiet"],"sensors":{"cpu_temp":60,"gpu_temp":50},"curves":{"quiet":[{"fan":"GPU","temp":[1,2,3,4,5,6,7,8],"pwm":[9,10,11,12,13,14,15,16]}]}}}`
	var resp response
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	s := convert(resp)
	if s.Profile != backend.ProfileQuiet || s.Sensors.CPUTemp != 60 {
		t.Errorf("snapshot is %+v", s)
	}
	if c := s.Curves[backend.ProfileQuiet][0]; c.Fan != "GPU" || c.Temp[7] != 8 || c.PWM[7] != 16 {
		t.Errorf("curve is %+v", c)
	}
}
