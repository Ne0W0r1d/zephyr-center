#!/bin/sh
# SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later
#
# build-icons.sh regenerates every application and tray icon from the
# source SVG, then rebuilds the Windows resource object. The generated
# files are committed, so a normal build does not need ImageMagick,
# Pillow or go-winres.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
src=${1:-"$root/data/io.github.ne0w0r1d.ZephyrCenter.svg"}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

magick -background none -density 384 "$src" -resize 1024x1024 -depth 8 -strip "$tmp/master.png"

for size in 16 24 32 48 64 128 256 512; do
  out="$root/data/icons/hicolor/${size}x${size}/apps/io.github.ne0w0r1d.ZephyrCenter.png"
  mkdir -p "$(dirname "$out")"
  magick "$tmp/master.png" -filter Lanczos -resize "${size}x${size}" -depth 8 -strip "$out"
done

mkdir -p "$root/data/icons/hicolor/scalable/apps"
cp "$src" "$root/data/icons/hicolor/scalable/apps/io.github.ne0w0r1d.ZephyrCenter.svg"
if [ "$(realpath "$src")" != "$(realpath "$root/data/io.github.ne0w0r1d.ZephyrCenter.svg")" ]; then
  cp "$src" "$root/data/io.github.ne0w0r1d.ZephyrCenter.svg"
fi

magick "$tmp/master.png" -filter Lanczos -resize 64x64 -depth 8 -strip \
  "$root/internal/appicon/tray.png"
install -m644 "$root/data/icons/hicolor/256x256/apps/io.github.ne0w0r1d.ZephyrCenter.png" \
  "$root/internal/appicon/app.png"

python3 - "$tmp/master.png" "$root/data/windows/zephyr-center.ico" <<'PY'
import sys
from PIL import Image

src, dst = sys.argv[1:]
Image.open(src).convert("RGBA").save(
    dst,
    format="ICO",
    sizes=[(16, 16), (20, 20), (24, 24), (32, 32), (40, 40), (48, 48),
           (64, 64), (128, 128), (256, 256)],
)
PY

(cd "$root/data/windows" && go run github.com/tc-hib/go-winres@v0.3.3 make \
  --in winres.json --arch amd64 --out ../../cmd/zephyr-center/rsrc)

echo "regenerated the icons and Windows resources"
