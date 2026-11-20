package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lichenghsu/synccraft/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := application.New(application.Options{
		Name:        "SyncCraft",
		Description: "Database schema & data compare tool",
		Services: []application.Service{
			application.NewService(app.NewHelloService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "SyncCraft",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
