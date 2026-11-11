// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build windows

package main

import (
	"github.com/Ne0W0r1d/zephyr-center/internal/backend"
	"github.com/Ne0W0r1d/zephyr-center/internal/backend/ghelper"
)

// newWindowsBackend returns the G-Helper bridge backend.
func newWindowsBackend() backend.Backend { return ghelper.New("") }
