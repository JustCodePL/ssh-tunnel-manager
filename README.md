# SSH Tunnel Manager

A lightweight desktop app for managing SSH tunnels with local port forwarding. Lives in the system tray — no browser, no cloud, no config servers.

Built with Go + [Wails v2](https://wails.io) + Svelte. Ships as a single binary (Windows: NSIS installer).

## Features

- **System tray first** — connect, disconnect, and copy port addresses without opening a window
- **Pure-Go SSH** — no dependency on an installed `ssh` binary
- **Port forwarding** — multiple local→remote mappings per tunnel, with one-click clipboard copy from the tray
- **Key authentication** — RSA, ECDSA, Ed25519; encrypted keys prompt for a passphrase once and store it in the OS keychain
- **ProxyCommand / ProxyJump** — full support for bastion hosts
- **Groups** — organise tunnels into named groups with Connect All / Disconnect All
- **Import from SSH config** — parse `~/.ssh/config` and import selected hosts in one step
- **Export to SSH config** — write selected tunnels back to a standard SSH config file
- **Auto-reconnect** — tunnels reconnect automatically on network interruption
- **Start on login** — optional autostart via Windows registry / macOS Launch Agent / Linux `.desktop` entry
- **OS keychain** — passphrases stored in Windows Credential Manager, macOS Keychain, or Linux Secret Service
- **Auto-update** — checks GitHub Releases on startup; download and install from the Settings panel or the tray menu

## Download

Get the latest release from the [Releases page](https://github.com/JustCodePL/ssh-tunnel-manager/releases).

| Platform | File | Notes |
|----------|------|-------|
| Windows | `ssh-tunnel-manager-amd64-installer.exe` | NSIS installer, recommended |
| Windows | `ssh-tunnel-manager-windows-amd64.zip` | Portable `.exe`, no installer |
| macOS | `ssh-tunnel-manager-darwin-arm64.dmg` | Disk image (Apple Silicon) |
| Linux | `ssh-tunnel-manager-linux-amd64.tar.gz` | Standalone binary |

## Installation

**Windows** — run the NSIS installer. It places the app in `Program Files`, creates a Start Menu entry, and registers an uninstaller.

**macOS** — open the DMG and drag `SSH Tunnel Manager.app` to `/Applications`. On first launch macOS may show a security prompt — open System Settings → Privacy & Security and click *Open Anyway*.

**Linux** — extract the binary to `~/.local/bin/` or `/usr/local/bin/` and run it. For autostart, enable *Start on login* in Settings.

#### Portless mode on Linux (privileged ports)

Portless forwards reach a service over a `*.ssh-local` domain instead of a `127.0.0.1:port` pair. When a forward binds a **privileged port** (below `1024`, e.g. `:80`), Linux requires the `CAP_NET_BIND_SERVICE` capability on the binary — unprivileged processes are otherwise denied with `bind: permission denied`. macOS and Windows have no such restriction, so this only applies to Linux.

You have two ways to grant it:

- **In-app (recommended).** The first time a privileged-port portless forward fails to bind, a banner appears with an **[ AUTHORIZE ]** button. Clicking it runs `pkexec setcap` and you approve the PolicyKit dialog once. Linux only applies a file capability at process start, so the banner then offers **[ RESTART NOW ]** — after the restart the forward binds normally.
- **Manually.** Run the command shown in the banner (and logged), pointing at your installed binary:

  ```sh
  sudo setcap 'cap_net_bind_service=+ep' ~/.local/bin/ssh-tunnel-manager
  ```

> **Re-apply after updates.** Capabilities are an attribute of the binary's inode, so an in-app update that replaces the binary drops the capability. The updater detects this and tries to restore it automatically (you may see one more PolicyKit prompt); if that fails — e.g. on a headless box without `pkexec` — the next failed bind shows the banner again with the exact `setcap` command.
>
> Distro packagers installing to a system path can ship `build/linux/pl.justcode.ssh-tunnel-manager.policy` to `/usr/share/polkit-1/actions/` for a friendlier authorization prompt.

## Usage

### System Tray

The tray icon is the primary interface. Left-click opens the manager window; right-click shows the context menu.

The icon colour indicates aggregate tunnel state:
- **Grey** — all tunnels disconnected
- **Green** — at least one tunnel connected
- **Red** — at least one tunnel in error state

Tunnels that have a colour assigned show coloured dots overlaid on the icon.

Each tunnel entry in the menu has a submenu with Connect / Disconnect and all local port addresses. Clicking a port address copies `127.0.0.1:<port>` to the clipboard.

When an update is available, an **⬆ Update to vX.Y.Z** item appears above the separator at the bottom of the menu.

### Manager Window

Open via the tray left-click or *Show Window*. From here you can:

- Add, edit, or delete tunnels
- See live status badges (Connecting / Connected / Error / Disconnected)
- Import tunnels from an SSH config file
- Export selected tunnels to an SSH config file

In **Settings → Startup**, *Start on login* enables OS autostart. *Start
minimized on login* is enabled by default so the app stays in the system tray
without taking focus after sign-in; disable it to open the manager window
automatically instead.

### Adding a Tunnel

Click **+ Add Tunnel** and fill in:

| Field | Description |
|-------|-------------|
| Name | Friendly label shown in the tray and window |
| Host | SSH server hostname or IP |
| Port | SSH port (default 22) |
| User | SSH username |
| Key file | Path to your private key (`.pem`, `id_ed25519`, etc.) |
| Group | Optional group name for organising tunnels |
| Color | Optional hex colour for the tray dot indicator |
| Auto-connect | Connect this tunnel automatically on app start |
| Port forwards | One or more `localPort → remoteHost:remotePort` mappings |
| ProxyCommand | e.g. `ssh -W %h:%p bastion` |
| ProxyJump | e.g. `user@bastion.example.com` |

### Authentication

The app uses public-key authentication only. Point the *Key file* field at your private key file. If the key is passphrase-protected, you will be prompted once per session; the passphrase is then stored in the OS keychain so subsequent launches are seamless.

### Auto-Update

On every startup the app silently checks GitHub Releases. The update channel can
be selected in **Settings → Updates**:

- **Stable** (default) receives full releases only
- **Beta** receives prereleases as well as full releases, so a beta installation
  automatically advances to the final stable build when it is published

Switching from Beta back to Stable automatically installs the newest full stable
release, even when its version is lower than the currently installed beta. The
stable build replaces the beta in place and restarts only after the old process
has exited. A single-instance lock prevents both channels from running side by
side.

If a newer version is found:

- A blue dot appears on the Settings gear icon in the manager window
- An **⬆ Update to vX.Y.Z** item appears in the tray menu
- A banner appears in **Settings → Updates**

Click **Install & Restart** (or the tray item) to download the installer and apply the update. The app quits automatically so the installer can replace the binary. On Windows, the NSIS installer runs silently in the background.

## Configuration

Tunnels are stored directly in your **`~/.ssh/config`** file as standard `Host` blocks. This means:

- Tunnels you add in the app are immediately visible to `ssh`, `git`, and any other tool that reads SSH config.
- Tunnels already in `~/.ssh/config` are loaded automatically on first launch.
- The file is plain text — you can hand-edit it at any time; the app re-reads it on the next operation.

App-specific metadata (port forwards, groups, colours, auto-connect) is stored as comments inside each `Host` block so it round-trips cleanly without breaking other SSH clients.

Passphrases are **never** written to disk; they go to the OS keychain (Windows Credential Manager / macOS Keychain / Linux Secret Service).

## Development

### Prerequisites

- Go 1.23+
- Node.js 20+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0`
- **Linux only**: `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev`
- **Windows installer builds**: [NSIS](https://nsis.sourceforge.io/)

### Running in dev mode

```bash
git clone https://github.com/JustCodePL/ssh-tunnel-manager
cd ssh-tunnel-manager
wails dev
```

Hot-reload is active for both the Go backend and the Svelte frontend.

### Running tests

```bash
go test ./internal/... -v
```

### Linting

```bash
golangci-lint run ./...
```

## Building

```bash
# Linux / macOS
wails build

# Windows — produces both ssh-tunnel-manager.exe and the NSIS installer
wails build -nsis
```

Output lands in `build/bin/`.

### Version

Local builds use `wails.json → info.productVersion`. Release builds inject the
full tag version into the application and keep the operating-system package
version numeric for Windows and macOS.

To publish a stable release:

1. Commit the release changes
2. Create an annotated tag such as `v1.1.0`. Use the first line of the tag
   message as the GitHub Release title, followed by a blank line and the release
   notes.
3. Push the tag: `git push origin v1.1.0`

To publish a beta release, use an annotated SemVer prerelease tag:

```bash
git tag -a v1.1.0-beta.1 \
  -m "Portless reliability and startup improvements" \
  -m "- Preserve automatic Portless forwards
- Explain server-side forwarding blocks"
git push origin v1.1.0-beta.1
```

The first paragraph becomes the per-version title shown on GitHub and on the
project site. The remaining paragraphs become the release description. Releases
without a custom title fall back to `Beta release`, `Latest stable release`, or
`Stable release` on the English site and their Polish equivalents.

Subsequent betas increment the suffix (`beta.2`, `beta.3`, …). Promote a tested
beta by tagging the chosen commit with the matching stable version, for example
`v1.1.0` after `v1.1.0-beta.3`.

GitHub Actions accepts only `vMAJOR.MINOR.PATCH` and
`vMAJOR.MINOR.PATCH-beta.N`, builds all platform artifacts, and marks beta tags
as GitHub prereleases without replacing the latest stable release.

## Project Structure

```
├── app.go                  # Wails-exposed methods (Go ↔ frontend bridge)
├── version.go              # Version variable, embedded from wails.json
├── main.go                 # Wails bootstrap
├── internal/
│   ├── ssh/                # Pure-Go SSH connections and port forwarding
│   ├── config/             # JSON persistence and data models
│   ├── tray/               # System tray icon, badge, menu
│   ├── updater/            # GitHub Releases update checker + platform installers
│   ├── autostart/          # Platform autostart (registry / launch agent / .desktop)
│   ├── keychain/           # OS keychain passphrase storage
│   └── sshconfig/          # SSH config file parser and writer
├── frontend/src/
│   ├── App.svelte           # Root component, header, modals
│   ├── components/          # TunnelList, TunnelCard, TunnelForm, SettingsPanel, …
│   └── stores/              # Svelte stores (tunnels, theme)
└── build/
    ├── windows/installer/   # NSIS scripts
    └── darwin/              # macOS plist files
```

## License

MIT — see [LICENSE](LICENSE) for details.
