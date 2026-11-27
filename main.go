package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lichenghsu/synccraft/internal/app"
	"github.com/lichenghsu/synccraft/internal/conn"
	"github.com/lichenghsu/synccraft/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	storePath, err := store.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	st := store.New(storePath, store.Keyring{Service: "synccraft"})
	pool := conn.NewPool()
	files := app.NewFileService()
	connections := app.NewConnectionService(files, st, pool)
	schema := app.NewSchemaService(st, pool)
	compare := app.NewCompareService(st, pool)

	a := application.New(application.Options{
		Name:        "SyncCraft",
		Description: "Database schema & data compare tool",
		Services: []application.Service{
			application.NewService(files),
			application.NewService(connections),
			application.NewService(schema),
			application.NewService(compare),
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
