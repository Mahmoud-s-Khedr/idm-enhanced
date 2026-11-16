package main

import (
	"context"
	"log"
	"runtime"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/frontend"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := "Linux socket ready"
	if runtime.GOOS != "linux" {
		state = "Browser IPC deferred to M5 on this platform"
	}
	service := desktop.New(state)
	if runtime.GOOS == "linux" {
		listener, e := ipc.Listen()
		if e != nil {
			log.Fatal(e)
		}
		defer func() { _ = listener.Close() }()
		go func() {
			if e := ipc.Serve(ctx, listener, service.Accept); e != nil {
				log.Print(e)
			}
		}()
	}
	app := application.New(application.Options{Name: "IDM Enhanced — M0", Services: []application.Service{application.NewService(service)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Assets)}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "IDM Enhanced — Platform spike", Width: 880, Height: 640, URL: "/"})
	if e := app.Run(); e != nil {
		log.Print(e)
	}
}
