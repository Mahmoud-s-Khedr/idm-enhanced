package main

import (
	"context"
	"log"
	"os"
	"runtime"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/frontend"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := "Linux socket ready"
	if runtime.GOOS != "linux" {
		state = "Browser IPC deferred to M5 on this platform"
	}
	service := desktop.New(state)
	if runtime.GOOS == "linux" {
		listener, err := ipc.Listen()
		if err != nil {
			return err
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := ipc.Serve(ctx, listener, service.Accept); err != nil {
				service.Disconnect()
				log.Print(err)
			}
		}()
		defer func() { cancel(); _ = listener.Close(); <-done }()
	}
	app := application.New(application.Options{Name: "IDM Enhanced — M0", Services: []application.Service{application.NewService(service)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Assets)}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "IDM Enhanced — Platform spike", Width: 880, Height: 640, URL: "/"})
	return app.Run()
}
