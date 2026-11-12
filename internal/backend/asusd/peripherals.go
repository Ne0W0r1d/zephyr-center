// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// readPeripherals lists the external ASUS and Logitech HID devices the
// kernel knows about, and merges in the battery some wireless receivers
// expose through power_supply. It is a read-only inventory: programming
// mouse DPI or per-device RGB needs each model's HID protocol.
func readPeripherals() []backend.Peripheral {
	byName := map[string]*backend.Peripheral{}

	// Wireless mice and headsets expose their battery as a HID-backed
	// power supply, both directly and through a Logitech receiver.
	patterns := []string{
		sysfsPath("class/power_supply/hid-*"),
		sysfsPath("class/power_supply/hidpp_battery_*"),
	}
	for _, pattern := range patterns {
		dirs, _ := filepath.Glob(pattern)
		for _, dir := range dirs {
			capacity, ok := readSysfsInt(filepath.Join(dir, "capacity"))
			if !ok {
				continue
			}
			name := peripheralName(dir)
			p := findPeripheral(byName, name)
			if p == nil {
				p = &backend.Peripheral{
					Name:       name,
					Kind:       peripheralKind(name),
					Connection: "wireless",
					Battery:    -1,
					Path:       dir,
				}
				byName[peripheralKey(name)] = p
			}
			p.Battery = capacity
			p.Status, _ = readSysfsString(filepath.Join(dir, "status"))
			if p.Path == "" {
				p.Path = dir
			}
		}
	}

	// Wired and Bluetooth devices do not always have a battery node, but
	// their HID identity still makes them useful in the inventory.
	dirs, _ := filepath.Glob(sysfsPath("bus/hid/devices/*"))
	for _, dir := range dirs {
		props := readUevent(filepath.Join(dir, "uevent"))
		name := props["HID_NAME"]
		vendor, product := hidIdentity(props["HID_ID"])
		if name == "" || builtInHID(name, props["HID_PHYS"], vendor, product) {
			continue
		}
		vendorName := peripheralVendor(vendor)
		if vendorName == "" {
			continue
		}
		p := findPeripheral(byName, name)
		if p == nil {
			p = &backend.Peripheral{Name: name, Battery: -1}
			byName[peripheralKey(name)] = p
		}
		// The HID name is usually fuller than the receiver's model name.
		p.Name = name
		p.Vendor = vendorName
		p.Product = product
		p.Kind = peripheralKind(name)
		p.Path = dir
		p.Connection = connectionForHID(props["HID_PHYS"])
	}

	out := make([]backend.Peripheral, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// peripheralName builds a readable name from the power-supply naming
// convention: manufacturer, model, then the node name.
func peripheralName(dir string) string {
	manufacturer, _ := readSysfsString(filepath.Join(dir, "manufacturer"))
	model, _ := readSysfsString(filepath.Join(dir, "model_name"))
	name := strings.TrimSpace(strings.Join(nonEmpty(manufacturer, model), " "))
	if name == "" {
		name = filepath.Base(dir)
	}
	return name
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// readUevent parses the KEY=VALUE pairs the kernel writes for a device.
func readUevent(path string) map[string]string {
	body, ok := readSysfsString(path)
	if !ok {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			out[key] = value
		}
	}
	return out
}

// hidIdentity splits HID_ID, whose form is bus:vendor:product.
func hidIdentity(id string) (vendor, product string) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return "", ""
	}
	return normalizeHex(parts[1]), normalizeHex(parts[2])
}

func normalizeHex(s string) string {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%04X", n)
}

func peripheralVendor(id string) string {
	switch id {
	case "0B05":
		return "ASUS"
	case "046D":
		return "Logitech"
	default:
		return ""
	}
}

// builtInHID keeps internal laptop controllers out of the external
// peripheral list. i2c HID devices are built in; the USB bus is where
// receivers and wired peripherals live.
func builtInHID(name, phys, vendor, product string) bool {
	lower := strings.ToLower(name)
	for _, word := range []string{"touchpad", "touch pad", "camera", "sensor", "hub", "wireless radio"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	// 19B6 is the laptop keyboard/backlight collection, not a detachable
	// keyboard. Its HID name also carries the N-KEY marker.
	if vendor == "0B05" && (product == "19B6" || strings.Contains(lower, "n-key")) {
		return true
	}
	phys = strings.ToLower(phys)
	return strings.HasPrefix(phys, "i2c-") || strings.HasPrefix(phys, "pci-")
}

func peripheralKind(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "mouse"):
		return "mouse"
	case strings.Contains(lower, "keyboard"):
		return "keyboard"
	case strings.Contains(lower, "headset") || strings.Contains(lower, "headphone"):
		return "headset"
	case strings.Contains(lower, "receiver"):
		return "receiver"
	default:
		return "hid"
	}
}

func connectionForHID(phys string) string {
	if strings.Contains(strings.ToLower(phys), "bluetooth") {
		return "bluetooth"
	}
	return "usb"
}

func peripheralKey(name string) string {
	lower := strings.ToLower(name)
	return strings.Join(strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}), "")
}

func findPeripheral(byName map[string]*backend.Peripheral, name string) *backend.Peripheral {
	key := peripheralKey(name)
	if p := byName[key]; p != nil {
		return p
	}
	for existing, p := range byName {
		if strings.Contains(existing, key) || strings.Contains(key, existing) {
			return p
		}
	}
	return nil
}
