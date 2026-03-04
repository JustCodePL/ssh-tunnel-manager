package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"ssh-tunnel-manager/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	store, err := config.NewStore()
	if err != nil {
		slog.Error("failed to initialize config store", "error", err)
		os.Exit(1)
	}

	app := NewApp(store)

	err = wails.Run(&options.App{
		Title:     "SSH Tunnel Manager",
		Width:     900,
		Height:    600,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:  &options.RGBA{R: 10, G: 10, B: 10, A: 1},
		HideWindowOnClose: true,
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		slog.Error("wails application error", "error", err)
		os.Exit(1)
	}
}
