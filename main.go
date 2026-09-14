// Port Authority desktop app: a Wails window around the same local API
// that pactl serve exposes. The frontend is a plain HTTP + WebSocket
// consumer of that API, so anything it can do a third-party UI can do.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// AppName is the product name. Rename here and in wails.json only.
const AppName = "Port Authority"

// Version is stamped at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     AppName,
		Width:     1280,
		Height:    860,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []any{app},
	})
	if err != nil {
		log.Fatalf("%s: %v", AppName, err)
	}
}
