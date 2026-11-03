#!/usr/bin/env bash
# Rebuilds frontend/vendor/ironrdp from a pinned IronRDP commit.
#
# Pinned to the head of Devolutions/IronRDP PR #1977, because the published npm
# packages do not advertise the Graphics Pipeline, which GNOME Remote Desktop
# requires. Move to the official packages once they include it.
#
# Needs git, node, and rustup. wasm-pack is installed into a temporary directory.
set -euo pipefail

REPO="https://github.com/juanjiTech/IronRDP.git"
COMMIT="af23bd5c427eb890686852cbd7a12f78681abbad"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="$ROOT/frontend/vendor/ironrdp"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone --quiet --filter=blob:none --no-checkout "$REPO" "$WORK/ironrdp"
git -C "$WORK/ironrdp" fetch --quiet origin "$COMMIT"
git -C "$WORK/ironrdp" checkout --quiet "$COMMIT"

export CARGO_HOME="$WORK/cargo"
export PATH="$CARGO_HOME/bin:$PATH"
(
  cd "$WORK/ironrdp"
  rustup target add wasm32-unknown-unknown
  cargo install wasm-pack --locked --quiet

  cd web-client/iron-remote-desktop
  npm ci --ignore-scripts --silent
  npx vite build

  cd ../iron-remote-desktop-rdp
  npm ci --ignore-scripts --silent
  npm run build
)

WEB="$WORK/ironrdp/web-client"
cp "$WEB/iron-remote-desktop/dist/iron-remote-desktop.js" "$TARGET/"
cp "$WEB/iron-remote-desktop/dist/index.d.ts" "$TARGET/iron-remote-desktop.d.ts"
cp "$WEB/iron-remote-desktop-rdp/dist/iron-remote-desktop-rdp.js" "$TARGET/"
cp "$WEB/iron-remote-desktop-rdp/dist/index.d.ts" "$TARGET/iron-remote-desktop-rdp.d.ts"
cp "$WORK/ironrdp/LICENSE-MIT" "$WORK/ironrdp/LICENSE-APACHE" "$TARGET/"

echo "Vendored IronRDP $COMMIT into $TARGET"
