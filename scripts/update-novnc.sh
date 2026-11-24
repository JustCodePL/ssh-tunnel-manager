#!/usr/bin/env bash
# Vendors the noVNC core library into frontend/vendor/novnc from the npm
# registry, pinned by version and checked against the registry's SHA-512.
#
# To move to another version, set VERSION and INTEGRITY from
# `npm view @novnc/novnc@<version> dist.integrity` and run this script.
#
# Needs npm, tar, shasum, xxd and base64.
set -euo pipefail

VERSION="1.7.0"
INTEGRITY="sha512-ucEJOx4T2avIRCleodk7YobZj5O2Ga2AeLfQ69A/yjG9HHba2+PDgwSkN3FttrmG+70ZGx21sElNFouK13RzyA=="

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="$ROOT/frontend/vendor/novnc"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

(cd "$WORK" && npm pack --silent "@novnc/novnc@$VERSION" >/dev/null)
TARBALL="$WORK/novnc-novnc-$VERSION.tgz"
ACTUAL="sha512-$(shasum -a 512 -b "$TARBALL" | cut -d' ' -f1 | xxd -r -p | base64 | tr -d '\n')"
if [ "$ACTUAL" != "$INTEGRITY" ]; then
  echo "noVNC $VERSION does not match the pinned checksum: $ACTUAL" >&2
  exit 1
fi

mkdir -p "$WORK/package"
tar -xzf "$TARBALL" -C "$WORK"
rm -rf "$TARGET/core" "$TARGET/vendor"
mkdir -p "$TARGET"
cp -R "$WORK/package/core" "$WORK/package/vendor" "$TARGET/"
cp "$WORK/package/LICENSE.txt" "$WORK/package/AUTHORS" "$TARGET/"
cp "$WORK/package/docs/LICENSE.MPL-2.0" "$TARGET/"

echo "Vendored noVNC $VERSION into $TARGET"
