// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build !windows

package ghelper

import "os/exec"

// hideWindow does nothing off Windows, where a child process has no window.
func hideWindow(cmd *exec.Cmd) {}
