# SSH Tunnel Manager

Cross-platform desktop app for managing SSH tunnels with port forwarding.
Go backend + Wails 3 + Svelte frontend. Single binary, no Electron.

## Quick Commands

- `cd frontend && npm run build && cd .. && go run .` — run the app
- `scripts/build-app.sh amd64` — release build into `build/bin`
- `wails3 generate bindings -ts -d frontend/bindings` — after changing bound methods
- `go test ./...` — unit and integration tests
- `cd frontend && npm run check && npm test` — types and frontend unit tests
- `cd frontend && npm run test:e2e` — real backend and frontend against a fake SSH server. All of these block a release.
- `STM_SMOKE_TEST=1 <app binary>` — starts the real webviews and exits 0 if they reach the backend
- `golangci-lint run ./...` — lint

## Architecture

- `app.go` — Wails-exposed methods (bridge Go ↔ frontend)
- `app_toolwindow.go`, `internal/windows/` — tool windows: the registry decides where a tool goes, the app creates the native window
- `app_settings.go`, `app_credentials.go`, `internal/prefs/` — settings and saved logins. Secrets stay in the keychain, names and assignments in prefs.json
- `internal/ssh/` — SSH connections via golang.org/x/crypto/ssh
- `internal/config/` — ~/.ssh/config persistence (read/write Host blocks), data models
- `internal/tray/` — System tray: icon, badge, colored dots, context menu
- `internal/autostart/` — Platform autostart (registry / launch agent / .desktop)
- `internal/keychain/` — OS keychain for passphrases and saved logins (SSH, RDP, VNC)
- `internal/rdp/` — Loopback WebSocket bridge between the IronRDP web client and RDP servers, plus RDP detection
- `frontend/vendor/ironrdp/` — Built IronRDP web client, pinned; rebuild with `scripts/build-ironrdp.sh`
- `frontend/src/` — Svelte + TypeScript + Tailwind. `App.svelte` is the main window, `ToolApp.svelte` a tool window (`/?window=<name>`)

## Key Decisions

- Pure Go SSH (golang.org/x/crypto/ssh) — no dependency on system `ssh` binary
- One goroutine per tunnel, managed via context.Context + sync.WaitGroup
- Config stored in ~/.ssh/config as standard Host blocks; app metadata (port forwards, groups, colour, auto-connect) stored as comments that round-trip cleanly
- A saved SSH password only goes to a server whose host key matches the one pinned on first use (`prefs.json`). Tunnels using only keys or the agent do not check host keys
- Passphrases stored in OS keychain (macOS Keychain / Windows Credential Manager / Linux Secret Service)
- System tray is the PRIMARY interface; manager window is secondary
- Use `127.0.0.1` not `localhost` for port forwarding (avoids IPv6 issues)
- Wails runtime events for tunnel status updates: `tunnel:status-changed`, `tunnel:error`
- `window.confirm` and `prompt` do nothing in the macOS webview: use `askConfirm` or an in-app dialog
- The RDP bridge checks the page origin; Wails 3 serves pages from `wails://localhost` on macOS

## Implementation Phases

1. Core SSH + Minimal UI (models, config, SSH tunnel, basic Wails window)
2. System Tray (icon, badge, colored dots, context menu, port copy)
3. Import/Export + Groups (SSH config parser, groups, grouped tray menu)
4. Resilience + Polish (auto-reconnect, autostart, keychain, themes)

## References

- See `.claude/skills/` for SSH and systray patterns (the Wails skill describes v2, check the v3 docs)
- See `.claude/rules/` for Go and frontend conventions
## Workflow — GitHub Projects

Projekt: https://github.com/orgs/JustCodePL/projects/1

Zasady przy pracy nad zadaniami:
- **Pewny fix** → push na `main` bezpośrednio, status → Done
- **Niepewny / wymaga testów** → branch + PR, status → In review
- Aktualizuj status w projekcie na bieżąco: Backlog → In progress → Done/In review
- Commituj z `Closes #N` żeby GitHub auto-zamknął issue
