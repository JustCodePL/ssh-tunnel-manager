package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed wails.json
var _wailsJSON []byte

// Version defaults to the productVersion in wails.json so that plain
// "wails build" produces a correctly versioned binary. Override at CI
// build time with ldflags if you want an exact release version injected:
//
//	wails build -ldflags "-X 'main.Version=1.2.3'"
var Version = "dev"

func init() {
	if Version != "dev" {
		return // already set via ldflags
	}
	var cfg struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(_wailsJSON, &cfg); err == nil && cfg.Info.ProductVersion != "" {
		Version = cfg.Info.ProductVersion
	}
}
