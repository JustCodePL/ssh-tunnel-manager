---
name: wails-dev
description: >
  Wails v2 desktop app development patterns. Use when creating or modifying
  Wails application structure, exposing Go methods to frontend, handling
  system tray, native dialogs, notifications, events, or building the app.
---

# Wails v2 Development

## Exposing Go to Frontend

Methods on the App struct are auto-exposed:
```go
// app.go
type App struct {
    ctx context.Context
}

func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
}

// Auto-available in frontend as App.GetTunnels()
func (a *App) GetTunnels() []config.TunnelConfig {
    return a.manager.GetAll()
}
```

## Frontend Bindings
```typescript
import { GetTunnels, ConnectTunnel } from '../wailsjs/go/main/App';
const tunnels = await GetTunnels();
```

## Runtime Events (Go → Frontend)
```go
import "github.com/wailsapp/wails/v2/pkg/runtime"
runtime.EventsEmit(a.ctx, "tunnel:status-changed", tunnelID, newStatus)
```
```typescript
import { EventsOn } from '../wailsjs/runtime/runtime';
EventsOn("tunnel:status-changed", (id, status) => { ... });
```

## System Tray

Wails v2 system tray uses the `github.com/energye/systray` package or
platform-native APIs. The tray icon must be dynamically generated
(draw colored dots + badge count onto a base icon).

## Build

- `wails dev` — development with hot-reload on frontend changes
- `wails build` — single binary with embedded frontend (go:embed)
- `wails build -platform darwin/amd64` — cross-compile