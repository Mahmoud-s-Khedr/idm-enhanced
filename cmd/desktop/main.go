package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/frontend"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/nativehost"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() (result error) {
	dir := flag.String("state-dir", "", "private application state directory")
	flag.Parse()
	if *dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*dir = filepath.Join(base, "idm-enhanced")
	}
	m, err := manager.Open(*dir, false, nil)
	if err != nil {
		return err
	}
	service := desktop.New("Browser bridge ready")
	if executable, err := os.Executable(); err == nil {
		suffix := ""
		if filepath.Ext(executable) == ".exe" {
			suffix = ".exe"
		}
		host := filepath.Join(filepath.Dir(executable), "native-host"+suffix)
		if err := nativehost.Register(host, false); err != nil {
			log.Print("Browser registration unavailable; run native-host --register after resolving an existing installation")
		}
	}
	service.SetTrayAvailable(trayAvailable())
	notifier := &completionNotifications{service: notifications.New(), desktop: service}
	session, err := manager.NewSession(m, func(j manager.Job) { notifier.send(j.ID) })
	if err != nil {
		return errors.Join(err, m.Close())
	}
	defer func() { result = errors.Join(result, session.Close()) }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := application.New(application.Options{Name: "IDM Enhanced", Services: []application.Service{application.NewService(service), application.NewService(notifier)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Assets)}, OnShutdown: func() {
		if err := session.Close(); err != nil {
			log.Print("Download shutdown failed")
			result = errors.Join(result, err)
		}
	}})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "IDM Enhanced", Width: 1120, Height: 800, MinWidth: 720, MinHeight: 560, URL: "/"})
	service.Attach(session, desktop.PlatformActions{
		ChooseDestination: func() (string, error) {
			return app.Dialog.SaveFile().SetMessage("Choose a new download destination").AttachToWindow(window).PromptForSingleSelection()
		},
		Open: func(path string, reveal bool) error {
			if reveal {
				return app.Env.OpenFileManager(path, true)
			}
			return app.Browser.OpenFile(path)
		}, Quit: app.Quit, AuthorizeNotifications: notifier.authorize,
	})
	{
		listener, err := ipc.Listen()
		if err != nil {
			return err
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := ipc.Serve(ctx, listener, service.Accept); err != nil {
				service.Disconnect()
				log.Print("Browser bridge disconnected")
			}
		}()
		defer func() { cancel(); _ = listener.Close(); <-done }()
	}
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon())
	tray.SetTooltip("IDM Enhanced — downloads continue while hidden")
	menu := application.NewMenu()
	menu.Add("Show downloads").OnClick(func(*application.Context) { window.Show(); window.Focus() })
	menu.Add("Run queued downloads").OnClick(func(*application.Context) {
		if err := session.Start("", false); err != nil {
			window.Show()
		}
	})
	menu.Add("Pause all downloads").OnClick(func(*application.Context) { session.PauseAll() })
	menu.Add("Quit safely").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { window.Show(); window.Focus() })
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		available := trayAvailable()
		log.Printf("Window close: tray available=%t", available)
		service.SetTrayAvailable(available)
		if available {
			window.Hide()
		} else {
			window.Minimise()
		}
	})
	app.Event.OnApplicationEvent(events.Common.SystemWillSleep, func(*application.ApplicationEvent) { session.Sleep() })
	app.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) { session.Wake() })
	configureSmoke(app, window, session)
	err = app.Run()
	return errors.Join(result, err)
}
