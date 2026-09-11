#!/bin/sh

set -eu

source_bundle="ssh-tunnel-manager.app"
target_bundle="SSH Tunnel Manager.app"
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
portless_plist="pl.justcode.ssh-tunnel-manager.portless.plist"
portless_helper="ssh-tunnel-manager-portless"

if [ ! -d "$source_bundle" ]; then
  echo "macOS app bundle not found: $PWD/$source_bundle" >&2
  exit 1
fi

# Wails does not clean build/bin by default, so remove only the previous
# generated bundle before replacing it with the freshly packaged application.
rm -rf -- "$target_bundle"
mv -- "$source_bundle" "$target_bundle"

# SMAppService only registers launchd property lists embedded at this exact
# bundle-relative location. Copy it before CI signs the complete app bundle.
mkdir -p -- "$target_bundle/Contents/Library/LaunchDaemons"
cp -- "$repo_root/build/darwin/$portless_plist" \
  "$target_bundle/Contents/Library/LaunchDaemons/$portless_plist"

# Build the minimal privileged executable for the same architecture as the
# Wails binary. This hook also runs for cross-architecture macOS builds.
main_binary="$target_bundle/Contents/MacOS/ssh-tunnel-manager"
case "$(file -b "$main_binary")" in
  *arm64*) helper_arch="arm64" ;;
  *x86_64*) helper_arch="amd64" ;;
  *) echo "cannot determine macOS helper architecture from $main_binary" >&2; exit 1 ;;
esac
mkdir -p -- "$target_bundle/Contents/Library/HelperTools"
(
  cd -- "$repo_root"
  CGO_ENABLED=1 GOOS=darwin GOARCH="$helper_arch" go build -tags portless_helper \
    -o "$PWD/build/bin/$target_bundle/Contents/Library/HelperTools/$portless_helper" \
    ./cmd/portless-helper
)
