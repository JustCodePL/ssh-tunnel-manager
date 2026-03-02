package main

// Version is set to "dev" locally; override at CI build time:
//
//	wails build -ldflags "-X 'main.Version=1.0.0'"
//
// Keep in sync with wails.json info.productVersion.
var Version = "dev"
