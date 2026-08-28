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
	"ssh-tunnel-manager/internal/dns"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if handled, err := updater.HandleInstallHelper(os.Args[1:]); handled {
		if err != nil {
			slog.Error("privileged update install failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// One-shot elevated Portless setup. Triggered when the app relaunches
	// itself via UAC / sudo / pkexec; it installs only the requested system
	// prerequisites and exits.
	setupRequested := false
	setupRequirements := dns.SetupRequirements{}
	for _, arg := range os.Args[1:] {
		switch arg {
		case dns.SetupArg:
			setupRequested = true
		case dns.PrivilegedRedirectArg:
			setupRequirements.PrivilegedPortRedirect = true
		}
	}
	if setupRequested {
		if err := dns.RunSetup(setupRequirements); err != nil {
			slog.Error("portless system setup failed", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

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

	p := prefsStore.Get()
	initWidth := p.WindowWidth
	initHeight := p.WindowHeight
	if initWidth < 400 {
		initWidth = 900
	}
	if initHeight < 300 {
		initHeight = 600
	}

	err = wails.Run(&options.App{
		Title:       "SSH Tunnel Manager",
		Width:       initWidth,
		Height:      initHeight,
		Frameless:   true,
		StartHidden: startHidden,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 10, A: 1},
		SingleInstanceLock: &options.SingleInstanceLock{
			// Stable and beta builds deliberately share one lock so switching
			// channels can never leave both variants running side by side.
			UniqueId: "com.wails.ssh-tunnel-manager",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				app.showWindow()
			},
		},
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
