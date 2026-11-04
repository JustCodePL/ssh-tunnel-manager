#!/usr/bin/env bash
# Builds the release app for this machine's platform into build/bin.
#
#   scripts/build-app.sh <arch> [version]
#
# arch is amd64 or arm64. version is the app version, such as 1.2.0-beta.3; by
# default the one in wails.json. macOS gets an app bundle, Windows the portable
# exe and the NSIS installer, Linux the binary. Signing happens later in CI.
set -euo pipefail

arch=${1:?usage: build-app.sh <arch> [version]}
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
default_version=$(sed -n 's/.*"productVersion": *"\([^"]*\)".*/\1/p' wails.json)
version=${2:-$default_version}
package_version=${version%%-*}
ldflags="-s -w -X main.Version=$version"

[ -d frontend/node_modules ] || (cd frontend && npm ci)
(cd frontend && npm run -s build)
mkdir -p build/bin

case "$(go env GOOS)" in
darwin)
  source_bundle=build/bin/ssh-tunnel-manager.app
  rm -rf "$source_bundle" "build/bin/SSH Tunnel Manager.app"
  mkdir -p "$source_bundle/Contents/MacOS" "$source_bundle/Contents/Resources"
  export MACOSX_DEPLOYMENT_TARGET=11.0
  clang_arch=$([ "$arch" = amd64 ] && echo x86_64 || echo arm64)
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" \
    CGO_CFLAGS="-arch $clang_arch -mmacosx-version-min=11.0" \
    CGO_LDFLAGS="-arch $clang_arch -mmacosx-version-min=11.0" \
    go build -tags production -trimpath -ldflags "$ldflags" \
    -o "$source_bundle/Contents/MacOS/ssh-tunnel-manager" .
  sed "s/@VERSION@/$package_version/g" build/darwin/Info.plist >"$source_bundle/Contents/Info.plist"

  iconset=$(mktemp -d)/iconfile.iconset
  mkdir -p "$iconset"
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" build/appicon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
    double=$((size * 2))
    sips -z "$double" "$double" build/appicon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
  done
  iconutil -c icns -o "$source_bundle/Contents/Resources/iconfile.icns" "$iconset"

  # Renames the bundle and adds the Portless helper.
  (cd build/bin && ../../scripts/rename-macos-app.sh)
  ;;
linux)
  CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
    go build -tags production -trimpath -ldflags "$ldflags" -o build/bin/ssh-tunnel-manager .
  ;;
windows)
  sed -e "s/{{.Info.ProductVersion}}/$package_version/g" \
    -e 's/{{.Info.CompanyName}}/JustCode/g' \
    -e 's/{{.Info.ProductName}}/SSH Tunnel Manager/g' \
    -e 's/{{.Info.Copyright}}/Copyright 2026 JustCode/g' \
    -e 's/{{.Info.Comments}}/SSH tunnels and server tools/g' build/windows/info.json >build/windows/info.rendered.json
  sed -i 's/{{.Name}}/ssh-tunnel-manager/g' build/windows/wails.exe.manifest
  go run github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 generate syso -arch "$arch" \
    -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest \
    -info build/windows/info.rendered.json -out "rsrc_windows_$arch.syso"
  trap 'rm -f "rsrc_windows_$arch.syso" build/windows/info.rendered.json' EXIT
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" \
    go build -tags production -trimpath -ldflags "$ldflags -H windowsgui" -o build/bin/ssh-tunnel-manager.exe .
  mkdir -p build/windows/installer/tmp
  curl -sSfL -o build/windows/installer/tmp/MicrosoftEdgeWebview2Setup.exe \
    "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
  (cd build/windows/installer && makensis \
    -DINFO_PRODUCTVERSION="$package_version" \
    "-DARG_WAILS_AMD64_BINARY=..\\..\\bin\\ssh-tunnel-manager.exe" project.nsi)
  ;;
*)
  echo "unsupported platform: $(go env GOOS)" >&2
  exit 1
  ;;
esac
