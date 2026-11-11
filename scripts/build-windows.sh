#!/bin/sh
# SPDX-License-Identifier: MPL-2.0 OR GPL-3.0-or-later
#
# build-windows.sh publishes the GPL-3.0 bridge and cross-compiles the app.
# It runs on Linux or on Windows with a POSIX shell.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out="$root/dist/windows"
version=${VERSION:-0.1.0}
mkdir -p "$out"

echo "building the bridge"
dotnet publish "$root/windows/bridge/ZephyrBridge.csproj" \
  -c Release -r win-x64 --self-contained false \
  -p:PublishSingleFile=true -p:Version="$version" -o "$out"

echo "building the app"
GOOS=windows GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$version" \
  -o "$out/zephyr-center.exe" "$root/cmd/zephyr-center"

echo "wrote $out"
ls -1 "$out"
