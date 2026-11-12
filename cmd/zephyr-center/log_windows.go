// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"
)

// redirectLog sends the app's log to a file. The Windows build is a GUI
// program with no console, so anything written to stderr would be lost,
// including the tray and bridge failures a user might need to report.
func redirectLog() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "zephyr-center")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, "zephyr-center.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(file)
}
