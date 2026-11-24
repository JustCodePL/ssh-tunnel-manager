# Vendored noVNC core library

The [noVNC](https://github.com/novnc/noVNC) core library, unmodified: `core/` and its bundled `vendor/pako/`.

- Source: npm package `@novnc/novnc` 1.7.0, checked against the registry's SHA-512 by the update script.
- Licence: MPL-2.0 for `core/` (`LICENSE.MPL-2.0`, `LICENSE.txt`), MIT for `vendor/pako/` (`vendor/pako/LICENSE`). The files stay unmodified and keep their licence notices, which is what MPL-2.0 asks of a larger work that includes them.
- Update: `scripts/update-novnc.sh`. Change `VERSION` and `INTEGRITY` there to move to another release.
- The app authenticates to the VNC server itself and offers this client a session without authentication, so the client never sees a password.
- Do not edit these files by hand.
