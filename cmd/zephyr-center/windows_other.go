// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
)

// newWindowsBackend stops the app where the Windows bridge is not built.
func newWindowsBackend() backend.Backend {
	fmt.Fprintln(os.Stderr, "zephyr-center: the G-Helper backend is built for Windows; use -demo elsewhere")
	os.Exit(1)
	return nil
}
