// SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later

// Package appicon holds the application icon bitmaps embedded in the
// binary, so the window and tray use the same artwork as the install.
package appicon

import _ "embed"

// AppPNG is the 256x256 window icon.
//
//go:embed app.png
var AppPNG []byte

// TrayPNG is the 64x64 notification-area icon.
//
//go:embed tray.png
var TrayPNG []byte
