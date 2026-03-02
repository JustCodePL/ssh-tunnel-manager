# SSH Tunnel Manager

Cross-platform desktop app for managing SSH tunnels with port forwarding.
Go backend + Wails v2 + Svelte frontend. Single binary, no Electron.

## Quick Commands

- `wails dev` — dev mode with hot-reload
- `wails build` — production binary
- `go test ./internal/... -v` — run tests
- `golangci-lint run ./...` — lint

## Architecture

- `app.go` — Wails-exposed methods (bridge Go ↔ frontend)
- `internal/ssh/` — SSH connections via golang.org/x/crypto/ssh
- `internal/config/` — ~/.ssh/config persistence (read/write Host blocks), data models
- `internal/tray/` — System tray: icon, badge, colored dots, context menu
- `internal/autostart/` — Platform autostart (registry / launch agent / .desktop)
- `internal/keychain/` — OS keychain for passphrases
- `frontend/src/` — Svelte + TypeScript + Tailwind

## Key Decisions

- Pure Go SSH (golang.org/x/crypto/ssh) — no dependency on system `ssh` binary
- One goroutine per tunnel, managed via context.Context + sync.WaitGroup
- Config stored in ~/.ssh/config as standard Host blocks; app metadata (port forwards, groups, colour, auto-connect) stored as comments that round-trip cleanly
- Passphrases stored in OS keychain (macOS Keychain / Windows Credential Manager / Linux Secret Service)
- System tray is the PRIMARY interface; manager window is secondary
- Use `127.0.0.1` not `localhost` for port forwarding (avoids IPv6 issues)
- Wails runtime events for tunnel status updates: `tunnel:status-changed`, `tunnel:error`

## Implementation Phases

1. Core SSH + Minimal UI (models, config, SSH tunnel, basic Wails window)
2. System Tray (icon, badge, colored dots, context menu, port copy)
3. Import/Export + Groups (SSH config parser, groups, grouped tray menu)
4. Resilience + Polish (auto-reconnect, autostart, keychain, themes)

## References

- See `.claude/skills/` for Wails, SSH, and systray patterns
- See `.claude/rules/` for Go and frontend conventions