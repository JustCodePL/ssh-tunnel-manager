#!/usr/bin/env bash
# Smoke-tests a published macOS release: downloads the DMG from GitHub, then
# checks its checksum, signature, notarization and stapling, and starts the app
# with STM_SMOKE_TEST=1 in an empty HOME. Touches nothing outside a temp
# directory and never installs the app.
#
# Usage: scripts/smoke-release.sh <tag> [arch]   e.g. v1.2.0-beta.7 arm64
# Needs: gh (authenticated), macOS with codesign, spctl and stapler.
set -euo pipefail

TAG="${1:?usage: $0 <tag> [arch]}"
ARCH="${2:-$(uname -m | sed 's/x86_64/amd64/')}"
REPO="${STM_REPO:-JustCodePL/ssh-tunnel-manager}"
TIMEOUT="${STM_SMOKE_TIMEOUT:-120}"
ASSET="ssh-tunnel-manager-darwin-${ARCH}.dmg"

WORK="$(mktemp -d)"
MNT="$WORK/mnt"
cleanup() {
  hdiutil detach -quiet "$MNT" 2>/dev/null || true
  chmod -R u+w "$WORK" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$MNT" "$WORK/app" "$WORK/home"

echo "== Download $ASSET from $TAG"
gh release download "$TAG" -R "$REPO" -p "$ASSET" -D "$WORK"
DMG="$WORK/$ASSET"
echo "SHA-256: $(shasum -a 256 "$DMG" | cut -d' ' -f1)"

echo "== Mount read-only"
hdiutil attach -nobrowse -readonly -mountpoint "$MNT" "$DMG" >/dev/null
APP_SRC="$(find "$MNT" -maxdepth 1 -name '*.app' | head -1)"
[ -n "$APP_SRC" ] || { echo "no .app in the DMG"; exit 1; }
echo "App: $(basename "$APP_SRC")"

echo "== codesign"
codesign --verify --deep --strict --verbose=2 "$APP_SRC"
echo "== spctl"
spctl -a -vv -t exec "$APP_SRC" 2>&1 | tee "$WORK/spctl.txt"
grep -q 'source=Notarized Developer ID' "$WORK/spctl.txt" || { echo "app is not notarized"; exit 1; }
echo "== stapler (DMG)"
xcrun stapler validate "$DMG"

APP="$WORK/app/$(basename "$APP_SRC")"
cp -R "$APP_SRC" "$WORK/app/"
PLIST="$APP/Contents/Info.plist"
VERSION="$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$PLIST")"
BIN="$APP/Contents/MacOS/$(/usr/libexec/PlistBuddy -c 'Print CFBundleExecutable' "$PLIST")"
echo "Version (Info.plist): $VERSION"
# The bundle version carries no prerelease suffix, so compare the base only.
EXPECTED="${TAG#v}"; EXPECTED="${EXPECTED%%-*}"
[ "$VERSION" = "$EXPECTED" ] || { echo "version mismatch: Info.plist $VERSION, tag expects $EXPECTED"; exit 1; }

echo "== Smoke test (limit ${TIMEOUT}s)"
set +e
STM_SMOKE_TEST=1 HOME="$WORK/home" perl -e 'alarm shift; exec @ARGV' "$TIMEOUT" "$BIN" >"$WORK/smoke.log" 2>&1
CODE=$?
set -e
echo "Exit code: $CODE"
tail -n 15 "$WORK/smoke.log"
[ "$CODE" -eq 0 ] || { echo "SMOKE TEST FAILED"; exit 1; }
echo "OK: $TAG ($ARCH)"
