#!/bin/sh

set -eu

source_bundle="ssh-tunnel-manager.app"
target_bundle="SSH Tunnel Manager.app"

if [ ! -d "$source_bundle" ]; then
  echo "macOS app bundle not found: $PWD/$source_bundle" >&2
  exit 1
fi

# Wails does not clean build/bin by default, so remove only the previous
# generated bundle before replacing it with the freshly packaged application.
rm -rf -- "$target_bundle"
mv -- "$source_bundle" "$target_bundle"
