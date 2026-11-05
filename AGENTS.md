# Repository instructions

## Project management: Notion

Notion is the source of truth for this project (`SSH Tunnel Manager`). If you do not know something, check Notion, and if it is not there, ask Artur instead of guessing.

- Project page: https://app.notion.com/p/40f23819443281528225c71b2b907880
- Tasks (`Zadania` database): https://app.notion.com/p/4b575a5bf5f440f5938864ac2052a6de
- `Changelog`: https://app.notion.com/p/40e23819443281ba84cadb2c244a33ad
- `Dokumentacja`: https://app.notion.com/p/40f2381944328193a408d579ce54d016

Rules:

- Track work in `Zadania`. Reuse an existing task when it covers the request; new tasks need no `JUS-` identifier.
- After every actual change, add an entry to `Changelog` under a heading with today's date: what changed, without secret values. Add nothing for read-only work.
- Keep project documentation in `Dokumentacja`.
- Linear is no longer used. Treat it, and any Linear link in old tasks and documents, as read-only history; do not create or update anything there.

### Task details

- Track every non-trivial bug fix or improvement for this repository in `Zadania`. It holds everything copied from Linear on 2026-11-04, under the same `JUS-` identifiers.
- The built-in RDP view keeps its own page, `SSH Tunnel Manager: wbudowany widok RDP przez IronRDP` (https://app.notion.com/p/40d238194432814f9011f808494b14ab).
- Keep the task's status aligned with the actual work state. A commit or completed implementation is not the same as a published release; record both states explicitly.
- Record the implementation commit SHA and the relevant verification results in the task. Do not add a GitHub commit link until the commit has been pushed and the link resolves.
- Set the task's release field to the SemVer version it ships in. Derive the next version from the latest release or Git tag: a patch increment for a backward-compatible bug fix, a minor increment for a backward-compatible feature.
- Never publish a tag, GitHub Release, or other externally visible release without Artur's explicit approval.

## Public GitHub issue tracking

- Track every public issue for this repository in the JustCodePL organization project at `https://github.com/orgs/JustCodePL/projects/1`.
- Deliver every issue-backed change and every bug fix through a pull request; do not commit those changes directly to the default branch.
- Link the pull request to its GitHub issue by using a closing keyword such as `Fixes #<number>` in the pull request description, unless the issue must intentionally remain open after merge; in that case, use an explicit non-closing issue link.
- Attach the pull request URL to the corresponding Notion task as soon as the pull request is created. Keep the GitHub issue, pull request, and Notion task mutually traceable.
- Keep the public issue updated automatically with concise English comments whenever its distribution state materially changes, including when the implementation is completed, when it is published in a beta or other prerelease, and when it reaches a stable release.
- State the exact verified availability in each comment. Include the commit SHA only after it is pushed and the GitHub commit link resolves; include the release version and public release link only after the corresponding release and artifacts have been published and verified.
- Do not describe a local commit or an untagged commit on a branch as released. Keep implementation, beta publication, and stable publication as explicit separate states.

## Distribution labels

- Keep the `Labels` of a Notion task aligned with the highest distribution channel that actually contains the change: `develop` when it is merged but unpublished, `beta` once a prerelease is published and verified, neither once it is in a stable release. Preserve unrelated labels.

## Project site and feature documentation

- The project site lives in `docs/` and is deployed to GitHub Pages on every merge to `main`. Each user-visible feature has a section on the Features page: `docs/features/index.html` (English) and `docs/pl/features/index.html` (Polish), with a stable anchor such as `#local-forwarding`.
- Every change to a user-visible feature (new feature, changed UI or behaviour, removal) updates the matching Features section in both languages in the same pull request. Add a section for a new feature, rewrite it for a changed one, and remove it for a removed one. Describe only what the code does; do not announce planned features.
- When the UI changes, list the screenshots that must be refreshed (file names under `docs/assets/screenshots/`) in the pull request description. Marketing regenerates the screenshot set with its demo script for each release and delivers files named `v<version>-...`; replace the files on the site and keep `alt`, `width` and `height` accurate.
- When a change affects claims elsewhere on the site, update them in the same pull request too: the home page, `docs/compare/` (EN/PL), the README (Features and Limitations) and `docs/sitemap.xml` for new pages.
- Each release has its own page on the site, written with the `changelog` skill: entries in `scripts/changelog/entries/<language>/<version>.html`, screenshots under `docs/assets/changelog/<version>/`, the version added to `docs/changelog-versions.js`. `node scripts/build-changelog.cjs` writes `docs/changelog/<version>/`, `docs/pl/changelog/<version>/`, both news pages and the sitemap; commit its output.
- Run `node --test docs/*.test.cjs` before opening the pull request; the tests check the analytics scripts, the Umami event attributes and the Features sections.

## Project overview

Cross-platform desktop app for managing SSH tunnels with port forwarding.
Go backend + Wails 3 + Svelte frontend. Single binary, no Electron.

### Quick Commands

- `cd frontend && npm run build && cd .. && go run .` — run the app
- `scripts/build-app.sh amd64` — release build into `build/bin`
- `wails3 generate bindings -ts -d frontend/bindings` — after changing bound methods
- `go test ./...` — unit and integration tests
- `cd frontend && npm run check && npm test` — types and frontend unit tests
- `cd frontend && npm run test:e2e` — real backend and frontend against a fake SSH server. All of these block a release.
- `STM_SMOKE_TEST=1 <app binary>` — starts the real webviews and exits 0 if they reach the backend
- `golangci-lint run ./...` — lint

### Architecture

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

### Key Decisions

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

### Implementation Phases

1. Core SSH + Minimal UI (models, config, SSH tunnel, basic Wails window)
2. System Tray (icon, badge, colored dots, context menu, port copy)
3. Import/Export + Groups (SSH config parser, groups, grouped tray menu)
4. Resilience + Polish (auto-reconnect, autostart, keychain, themes)

### References

- See `.claude/skills/` for SSH and systray patterns (the Wails skill describes v2, check the v3 docs)
- See `.claude/rules/` for Go and frontend conventions
## Workflow — GitHub Projects

Projekt: https://github.com/orgs/JustCodePL/projects/1

Zasady przy pracy nad zadaniami:
- **Pewny fix** → push na `main` bezpośrednio, status → Done
- **Niepewny / wymaga testów** → branch + PR, status → In review
- Aktualizuj status w projekcie na bieżąco: Backlog → In progress → Done/In review
- Commituj z `Closes #N` żeby GitHub auto-zamknął issue
