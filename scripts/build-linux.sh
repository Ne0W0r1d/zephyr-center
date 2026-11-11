#!/bin/sh
# SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later
#
# build-linux.sh builds the app and stages the files that ship in the Linux
# bundle: the binary, the desktop entry, the icon and the licences.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out="$root/dist/linux"
version=${VERSION:-0.1.0}
mkdir -p "$out"

echo "building the app"
CGO_ENABLED=${CGO_ENABLED:-0} go build -trimpath \
  -ldflags "-s -w -X main.version=$version" \
  -o "$out/zephyr-center" "$root/cmd/zephyr-center"

install -Dm644 "$root/data/io.github.ne0w0r1d.ZephyrCenter.desktop" \
  "$out/share/applications/io.github.ne0w0r1d.ZephyrCenter.desktop"
install -Dm644 "$root/data/io.github.ne0w0r1d.ZephyrCenter.svg" \
  "$out/share/icons/hicolor/scalable/apps/io.github.ne0w0r1d.ZephyrCenter.svg"
install -Dm644 "$root/LICENSE" "$out/LICENSE"
install -Dm644 "$root/NOTICE" "$out/NOTICE"
install -Dm644 "$root/README.md" "$out/README.md"

echo "wrote $out"
ls -1 "$out"
