// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

//go:build !windows

package main

// redirectLog leaves the log on stderr, which a terminal or the journal
// already collects.
func redirectLog() {}
