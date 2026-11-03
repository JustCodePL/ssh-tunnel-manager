#!/usr/bin/env bash
# Runs an isolated copy of the app from this checkout and saves a screenshot of
# its window every second, for the changelog. macOS only.
#
#   scripts/changelog-capture.sh <scenario.sshconfig> <output-dir> [seconds]
#
# The copy uses a temporary HOME and its own single-instance lock, so it can run
# next to an installed app without touching its tunnels or ~/.ssh/config. Frames
# identical to the previous one are dropped.
set -euo pipefail

scenario=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
out=$2
seconds=${3:-30}
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/stm-capture.XXXXXX")
app_pid=""

cleanup() {
  # The app ignores SIGTERM while it stays in the tray.
  [ -n "$app_pid" ] && kill -9 "$app_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

mkdir -p "$out" "$work/home/.ssh"
out=$(cd "$out" && pwd)
chmod 700 "$work/home/.ssh"
cp "$scenario" "$work/home/.ssh/config"

cd "$repo"
[ -d frontend/node_modules ] || (cd frontend && npm ci)
(cd frontend && npm run -s build) >/dev/null

sed 's/com.wails.ssh-tunnel-manager/com.wails.ssh-tunnel-manager.capture/' main.go >"$work/main.go"
grep -q 'ssh-tunnel-manager.capture' "$work/main.go"
printf '{"Replace":{"%s/main.go":"%s/main.go"}}' "$repo" "$work" >"$work/overlay.json"
CGO_LDFLAGS="-framework UniformTypeIdentifiers" \
  go build -tags desktop,production -overlay "$work/overlay.json" -ldflags "-w -s" -o "$work/stm" . 2>/dev/null

cat >"$work/winid.swift" <<'SWIFT'
import CoreGraphics
let pid = Int(CommandLine.arguments[1])!
let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
for w in windows where (w[kCGWindowOwnerPID as String] as? Int) == pid && (w[kCGWindowLayer as String] as? Int) == 0 {
  print(w[kCGWindowNumber as String]!)
  break
}
SWIFT
swiftc -o "$work/winid" "$work/winid.swift"

(cd "$work" && HOME="$work/home" exec ./stm) >"$work/run.log" 2>&1 &
app_pid=$!

previous=""
captured=0
for ((i = 1; i <= seconds; i++)); do
  sleep 1
  window=$("$work/winid" "$app_pid" || true)
  [ -n "$window" ] || continue
  frame="$out/$(printf '%03d' "$i").png"
  screencapture -x -o -l "$window" "$frame"
  hash=$(shasum "$frame" | cut -d' ' -f1)
  if [ "$hash" = "$previous" ]; then
    rm "$frame"
  else
    previous=$hash
    captured=$((captured + 1))
  fi
done

if [ "$captured" -eq 0 ]; then
  echo "No frames: the app window never appeared. Log:" >&2
  cat "$work/run.log" >&2
  exit 1
fi
echo "Saved $captured frames to $out"
echo "App log:"
cat "$work/home/.config/ssh-tunnel-manager/app.log" 2>/dev/null || true
