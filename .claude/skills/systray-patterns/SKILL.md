---
name: systray-patterns
description: >
  Cross-platform system tray patterns for Go desktop apps.
  Use when implementing tray icons, dynamic badges, colored indicator dots,
  context menus with submenus, clipboard operations, or native notifications.
---

# System Tray Patterns

## Dynamic Icon Generation

Generate tray icon programmatically using Go's `image` package:

1. Base icon (small tunnel/key symbol) — embed as PNG via go:embed
2. Draw colored dots for active tunnels (top area of icon)
3. Draw badge number (bottom-right) for active tunnel count
4. Convert to platform icon format (ICO on Windows, PNG on Linux/macOS)

Use `image/draw` and `image/color` packages. Regenerate icon on every
tunnel status change.

## Context Menu Structure

Hierarchical menu:
- Group headers (non-clickable, bold)
  - Tunnel entries with status icon (●) + submenu
    - Port entries: "Label (port)" — on click: copy to clipboard
    - Separator
    - Connect/Disconnect toggle
- Separator
- Connect All / Disconnect All
- Open Manager...
- Quit

## Clipboard
```go
import "golang.design/x/clipboard"
clipboard.Write(clipboard.FmtText, []byte("127.0.0.1:5432"))
```

## Native Notifications

Use Wails runtime or `github.com/gen2brain/beeep`:
```go
beeep.Notify("SSH Tunnel Manager", "Copied 127.0.0.1:5432", "")
```