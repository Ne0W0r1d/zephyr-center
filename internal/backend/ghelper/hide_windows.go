// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build windows

package ghelper

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps the bridge off the desktop. The app is a GUI program, so
// Windows would otherwise give the console helper a window of its own.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// createNoWindow is CREATE_NO_WINDOW, which starts a console program with no
// console at all rather than a hidden one.
const createNoWindow = 0x08000000
