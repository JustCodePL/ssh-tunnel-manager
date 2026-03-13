package main

import (
	"context"
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/prefs"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	startHidden := false
	for _, arg := range os.Args[1:] {
		if arg == "--hidden" {
			startHidden = true
		}
	}

	store, err := config.NewStore()
	if err != nil {
		slog.Error("failed to initialize config store", "error", err)
		os.Exit(1)
	}

	prefsStore, err := prefs.NewStore()
	if err != nil {
		slog.Error("failed to initialize prefs store", "error", err)
		os.Exit(1)
	}

	app := NewApp(store, prefsStore, startHidden)

	err = wails.Run(&options.App{
		Title:     "SSH Tunnel Manager",
		Width:     900,
		Height:    600,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 10, A: 1},
		OnBeforeClose: func(ctx context.Context) bool {
			if app.forceQuit {
				return false // always allow quit from tray
			}
			if app.GetCloseToTray() {
				runtime.WindowHide(ctx)
				return true // cancel the close — hide instead
			}
			return false // allow quit
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		slog.Error("wails application error", "error", err)
		os.Exit(1)
	}
}
