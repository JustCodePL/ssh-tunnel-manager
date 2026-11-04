#!/usr/bin/env bash
# Runs an isolated copy of the app from this checkout and saves a screenshot of
# each of its windows every second, for the changelog. macOS only.
#
#   scripts/changelog-capture.sh <scenario.sshconfig> <output-dir> [seconds] [driver.js]
#
# A driver is JavaScript added to the page for this copy only, for states that
# need clicks. In the scenario, @HOME@ is the real home directory and @BASTION@,
# @BASTION_USER@ and @RDP_TARGET@ come from $CHANGELOG_BASTION,
# $CHANGELOG_BASTION_USER and $CHANGELOG_RDP_TARGET. In the driver, @LOGIN@ and
# @PASSWORD@ are read from the login:password file in $CHANGELOG_LOGIN_FILE.
#
# The copy uses a temporary HOME and its own single-instance lock, so it can run
# next to an installed app without touching its tunnels or ~/.ssh/config. Frames
# identical to the previous one are dropped.
set -euo pipefail

scenario=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
out=$2
seconds=${3:-30}
driver=${4:-}
real_home=$HOME
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(cd "$(mktemp -d "${TMPDIR:-/tmp}/stm-capture.XXXXXX")" && pwd -P)
app_pid=""

cleanup() {
  # The app ignores SIGTERM while it stays in the tray.
  [ -n "$app_pid" ] && kill -9 "$app_pid" 2>/dev/null || true
  [ -f "$work/index.html" ] && cp "$work/index.html" "$repo/frontend/dist/index.html"
  rm -rf "$work"
}
trap cleanup EXIT

mkdir -p "$out" "$work/home/.ssh"
out=$(cd "$out" && pwd)
chmod 700 "$work/home/.ssh"
sed -e "s|@HOME@|$real_home|g" -e "s|@BASTION@|${CHANGELOG_BASTION:-}|g" \
  -e "s|@BASTION_USER@|${CHANGELOG_BASTION_USER:-}|g" -e "s|@RDP_TARGET@|${CHANGELOG_RDP_TARGET:-}|g" "$scenario" >"$work/home/.ssh/config"

cd "$repo"
[ -d frontend/node_modules ] || (cd frontend && npm ci)
(cd frontend && npm run -s build) >/dev/null 2>&1

if [ -n "$driver" ]; then
  cp frontend/dist/index.html "$work/index.html"
  python3 -I - "$driver" frontend/dist/index.html <<'PY'
import json, os, sys

driver_path, page_path = sys.argv[1:3]
login, password = "", ""
if os.environ.get("CHANGELOG_LOGIN_FILE"):
    login, password = open(os.environ["CHANGELOG_LOGIN_FILE"]).read().rstrip("\n").split(":", 1)
script = open(driver_path).read()
for token, value in (("@LOGIN@", login), ("@PASSWORD@", password)):
    script = script.replace('"%s"' % token, json.dumps(value))
page = open(page_path).read()
open(page_path, "w").write(page.replace("</body>", "<script>" + script + "</script></body>"))
PY
fi

sed 's/com.wails.ssh-tunnel-manager/com.wails.ssh-tunnel-manager.capture/' main.go >"$work/main.go"
grep -q 'ssh-tunnel-manager.capture' "$work/main.go"
printf '{"Replace":{"%s/main.go":"%s/main.go"}}' "$repo" "$work" >"$work/overlay.json"
go build -tags production -overlay "$work/overlay.json" -ldflags "-w -s" -o "$work/stm" . 2>/dev/null

cat >"$work/winid.swift" <<'SWIFT'
import CoreGraphics
import Foundation
let pid = Int(CommandLine.arguments[1])!
let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
for w in windows where (w[kCGWindowOwnerPID as String] as? Int) == pid && (w[kCGWindowLayer as String] as? Int) == 0 {
  print(w[kCGWindowNumber as String]!)
}
SWIFT
swiftc -o "$work/winid" "$work/winid.swift"

(cd "$work" && HOME="$work/home" exec ./stm) >"$work/run.log" 2>&1 &
app_pid=$!

previous=""
captured=0
for ((i = 1; i <= seconds; i++)); do
  sleep 1
  for window in $("$work/winid" "$app_pid" || true); do
    frame="$out/w$window-$(printf '%03d' "$i").png"
    # Fails for a moment while the window is still appearing.
    screencapture -x -o -l "$window" "$frame" 2>/dev/null || continue
    hash=$(shasum "$frame" | cut -d' ' -f1)
    if grep -qx "$window $hash" "$work/hashes" 2>/dev/null; then
      rm "$frame"
    else
      echo "$window $hash" >>"$work/hashes"
      captured=$((captured + 1))
    fi
  done
done

if [ "$captured" -eq 0 ]; then
  echo "No frames: the app window never appeared. Log:" >&2
  cat "$work/run.log" >&2
  exit 1
fi
echo "Saved $captured frames to $out"
echo "App log:"
cat "$work/home/.config/ssh-tunnel-manager/app.log" 2>/dev/null || true
