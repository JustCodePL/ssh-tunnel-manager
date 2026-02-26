Implement phase $ARGUMENTS of the SSH Tunnel Manager.

Before starting:
1. Read CLAUDE.md and check current codebase state
2. List what you will implement in this phase
3. Wait for my confirmation before writing code

After implementation:
1. Run `wails dev` to verify compilation
2. Run `go test ./internal/... -v` if tests exist
3. Summarize what was done and what's next

Phases:
- 1: Core SSH + Minimal UI (models, config store, SSH tunnel manager, basic Wails window with tunnel list and add form)
- 2: System Tray (dynamic icon with badge + colored dots, context menu with tunnel submenus, port copy to clipboard, notifications)
- 3: Import/Export + Groups (SSH config parser/generator, import preview UI, tunnel groups model + UI + tray)
- 4: Resilience + Polish (auto-reconnect with backoff, autostart 3 platforms, keychain, port conflict detection, dark/light theme)