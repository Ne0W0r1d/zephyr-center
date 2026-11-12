// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

package asusd

import (
	"path/filepath"
	"testing"
)

func TestReadPeripheralsMergesReceiverBattery(t *testing.T) {
	withSysfs(t)

	battery := filepath.Join(sysRoot, "class/power_supply/hidpp_battery_0")
	writeTestFile(t, filepath.Join(battery, "manufacturer"), "Logitech\n")
	writeTestFile(t, filepath.Join(battery, "model_name"), "G502 HERO\n")
	writeTestFile(t, filepath.Join(battery, "capacity"), "73\n")
	writeTestFile(t, filepath.Join(battery, "status"), "Discharging\n")

	logitech := filepath.Join(sysRoot, "bus/hid/devices/0003:046D:C08B.0001")
	writeTestFile(t, filepath.Join(logitech, "uevent"), ""+
		"HID_NAME=Logitech G502 HERO Gaming Mouse\n"+
		"HID_ID=0003:0000046D:0000C08B\n"+
		"HID_PHYS=usb-0000:00:14.0-3/input2\n")

	asus := filepath.Join(sysRoot, "bus/hid/devices/0003:0B05:1A72.0002")
	writeTestFile(t, filepath.Join(asus, "uevent"), ""+
		"HID_NAME=ROG Gladius III Wireless AimPoint\n"+
		"HID_ID=0003:00000B05:00001A72\n"+
		"HID_PHYS=usb-0000:00:14.0-4/input0\n")

	builtIn := filepath.Join(sysRoot, "bus/hid/devices/0018:0B05:0001.0003")
	writeTestFile(t, filepath.Join(builtIn, "uevent"), ""+
		"HID_NAME=ASUS Touchpad\n"+
		"HID_ID=0018:00000B05:00000001\n"+
		"HID_PHYS=i2c-ASUS0001:00\n")

	keyboard := filepath.Join(sysRoot, "bus/hid/devices/0003:0B05:19B6.0004")
	writeTestFile(t, filepath.Join(keyboard, "uevent"), ""+
		"HID_NAME=ASUSTeK Computer Inc. N-KEY\n"+
		"HID_ID=0003:00000B05:000019B6\n"+
		"HID_PHYS=usb-0000:00:14.0-5/input0\n")

	devices := readPeripherals()
	if len(devices) != 2 {
		t.Fatalf("peripherals are %+v, want the mouse and keyboard", devices)
	}
	if devices[0].Name != "Logitech G502 HERO Gaming Mouse" || devices[0].Battery != 73 {
		t.Errorf("Logitech entry is %+v, want the receiver battery merged in", devices[0])
	}
	if devices[0].Vendor != "Logitech" || devices[0].Connection != "usb" {
		t.Errorf("Logitech identity is %+v", devices[0])
	}
	if devices[1].Vendor != "ASUS" || devices[1].Battery != -1 {
		t.Errorf("ASUS entry is %+v, want no battery", devices[1])
	}
}
