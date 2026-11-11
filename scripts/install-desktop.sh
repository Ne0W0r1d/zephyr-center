#!/bin/sh
# SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later
#
# install-desktop.sh installs the binary, the icon and the desktop entry for
# the current user.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
prefix=${PREFIX:-$HOME/.local}
bindir="$prefix/bin"
appdir="$prefix/share/applications"
icondir="$prefix/share/icons/hicolor/scalable/apps"

go build -trimpath -o "$bindir/zephyr-center" "$root/cmd/zephyr-center"
install -Dm644 "$root/data/io.github.ne0w0r1d.ZephyrCenter.svg" "$icondir/io.github.ne0w0r1d.ZephyrCenter.svg"
install -Dm644 "$root/data/io.github.ne0w0r1d.ZephyrCenter.desktop" "$appdir/io.github.ne0w0r1d.ZephyrCenter.desktop"
update-desktop-database "$appdir" 2>/dev/null || true
gtk-update-icon-cache -f -t "$prefix/share/icons/hicolor" 2>/dev/null || true

echo "installed to $prefix"
