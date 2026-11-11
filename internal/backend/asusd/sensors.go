// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// sensors reads what asusd does not expose over D-Bus: the fans' speeds,
// the CPU package temperature and the battery.
type sensors struct {
	mu       sync.Mutex
	last     cpuTicks
	lastAt   time.Time
	usage    float64
	nvidiaAt time.Time
	nvidia   float64
}

type cpuTicks struct {
	idle  uint64
	total uint64
}

func newSensors() *sensors { return &sensors{} }

// read returns the live sensor values.
func (s *sensors) read() backend.Sensors {
	var out backend.Sensors
	out.CPUTemp = s.cpuTemp()
	out.GPUTemp = s.gpuTemp()
	out.CPURPM, out.GPURPM, out.MidRPM = fanRPMs()
	out.CPUUsage = s.cpuUsage()
	return out
}

// hwmonByName finds the hwmon directory whose name file says name.
func hwmonByName(name string) string {
	entries, err := os.ReadDir("/sys/class/hwmon")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		dir := filepath.Join("/sys/class/hwmon", e.Name())
		if b, err := os.ReadFile(filepath.Join(dir, "name")); err == nil && strings.TrimSpace(string(b)) == name {
			return dir
		}
	}
	return ""
}

// readInt reads an integer file, returning 0 when it is missing.
func readInt(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// fanRPMs reads the asus hwmon fans, as asusd's get_fan_rpms does.
func fanRPMs() (cpu, gpu, mid int) {
	dir := hwmonByName("asus")
	if dir == "" {
		return 0, 0, 0
	}
	return readInt(filepath.Join(dir, "fan1_input")),
		readInt(filepath.Join(dir, "fan2_input")),
		readInt(filepath.Join(dir, "fan3_input"))
}

// cpuTemp reads the package temperature from coretemp, else acpitz.
func (s *sensors) cpuTemp() float64 {
	for _, name := range []string{"coretemp", "k10temp", "zenpower", "acpitz"} {
		dir := hwmonByName(name)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == "temp1_input" {
				if v := readInt(filepath.Join(dir, e.Name())); v > 0 {
					return float64(v) / 1000
				}
			}
		}
	}
	return 0
}

// gpuTemp asks nvidia-smi for the discrete GPU's temperature, at most once
// every two seconds, then falls back to the asus hwmon if it has one.
func (s *sensors) gpuTemp() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.nvidiaAt) > 2*time.Second {
		s.nvidiaAt = time.Now()
		s.nvidia = nvidiaTemp()
	}
	if s.nvidia > 0 {
		return s.nvidia
	}
	dir := hwmonByName("asus")
	if dir == "" {
		return 0
	}
	for _, name := range []string{"temp2_input", "temp1_input"} {
		if v := readInt(filepath.Join(dir, name)); v > 0 {
			return float64(v) / 1000
		}
	}
	return 0
}

// nvidiaTemp runs nvidia-smi for the GPU temperature, returning 0 when it
// is absent.
func nvidiaTemp() float64 {
	out, err := runTimeout(800*time.Millisecond, "nvidia-smi", "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return 0
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	v, _ := strconv.ParseFloat(strings.TrimSpace(line), 64)
	return v
}

// cpuUsage computes the CPU load since the last call from /proc/stat.
func (s *sensors) cpuUsage() float64 {
	now, ok := readCPUTicks()
	if !ok {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, at := s.last, s.lastAt
	s.last, s.lastAt = now, time.Now()
	if at.IsZero() {
		return 0
	}
	total := float64(now.total - prev.total)
	idle := float64(now.idle - prev.idle)
	if total <= 0 {
		return s.usage
	}
	s.usage = clamp01((total - idle) / total * 100)
	return s.usage
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// readCPUTicks reads the aggregate line of /proc/stat.
func readCPUTicks() (cpuTicks, bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTicks{}, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		var t cpuTicks
		for i, f := range fields {
			v, _ := strconv.ParseUint(f, 10, 64)
			t.total += v
			if i == 3 || i == 4 {
				t.idle += v
			}
		}
		return t, true
	}
	return cpuTicks{}, false
}

// battery reads the battery's capacity, status, power and health.
func battery() backend.Battery {
	const dir = "/sys/class/power_supply/BAT0"
	if _, err := os.Stat(dir); err != nil {
		return backend.Battery{}
	}
	b := backend.Battery{
		Present:  true,
		Capacity: readInt(filepath.Join(dir, "capacity")),
		Status:   strings.TrimSpace(readString(filepath.Join(dir, "status"))),
		Cycles:   readInt(filepath.Join(dir, "cycle_count")),
	}
	if v := readInt(filepath.Join(dir, "power_now")); v != 0 {
		b.PowerW = float64(v) / 1e6
	}
	if v := readInt(filepath.Join(dir, "voltage_now")); v != 0 {
		if p := readInt(filepath.Join(dir, "current_now")); p != 0 {
			b.PowerW = float64(v) / 1e6 * float64(p) / 1e6
		}
	}
	// Health is the full charge over the design charge when the firmware
	// reports both, as energy_full and energy_full_design.
	full, design := readInt(filepath.Join(dir, "energy_full")), readInt(filepath.Join(dir, "energy_full_design"))
	if full == 0 || design == 0 {
		full, design = readInt(filepath.Join(dir, "charge_full")), readInt(filepath.Join(dir, "charge_full_design"))
	}
	if full > 0 && design > 0 {
		b.Health = float64(full) / float64(design) * 100
	}
	if b.Status == "Discharging" && b.PowerW > 0 {
		if now := readInt(filepath.Join(dir, "energy_now")); now > 0 {
			hours := float64(now) / 1e6 / b.PowerW
			b.TimeLeft = formatHours(hours)
		}
	}
	return b
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func formatHours(h float64) string {
	if h <= 0 || h > 24 {
		return ""
	}
	m := int(h * 60)
	return strconv.Itoa(m/60) + "h " + strconv.Itoa(m%60) + "m"
}
