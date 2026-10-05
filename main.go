package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"syncscope/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg, err := config.Open()
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(cfg)

	err = wails.Run(&options.App{
		Title:     "Syncscope",
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 39, B: 51, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About:    &mac.AboutInfo{Title: "Syncscope", Message: "Syncscope for Argo Apps\nDesktop app for Argo CD — Apache-2.0\nNot affiliated with the Argo project."},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
