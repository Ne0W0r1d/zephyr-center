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
    *'"snapshot"'*) echo '{"ok":true,"backend":"ghelper","version":"0.1.0","snapshot":{"connected":true,"model":"ROG Zephyrus G14","bios":"GA402.312","cpu":"AMD Ryzen 9","profile":"performance","profile_choices":["quiet","balanced","performance"],"charge_limit":80,"charge_supported":true,"sensors":{"cpu_temp":62,"gpu_temp":51,"cpu_rpm":3000,"gpu_rpm":2800},"battery":{"present":true,"capacity":71,"status":"Charging"},"gpu":{"supported":true,"mode":"hybrid","modes":["integrated","hybrid","ultimate"]},"features":{"panel_overdrive":{"supported":true,"value":1,"values":[0,1]}},"curves":{"balanced":[{"fan":"CPU","temp":[30,40,50,60,70,80,90,100],"pwm":[0,64,128,160,192,224,240,255]}]}}}' ;;
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
	curves := s.Curves[backend.ProfileBalanced]
	if len(curves) != 1 || curves[0].PWM[7] != 255 || curves[0].Temp[0] != 30 {
		t.Errorf("curves are %+v", curves)
	}
	if f := s.Features["panel_overdrive"]; !f.Supported || f.Value != 1 {
		t.Errorf("feature is %+v", f)
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
