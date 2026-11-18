//go:build desktop_smoke

package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed smoke.js
var smokeScript string

func configureSmoke(app *application.App, window *application.WebviewWindow, session *manager.Session) {
	config := os.Getenv("IDM_DESKTOP_SMOKE")
	if !json.Valid([]byte(config)) {
		panic("desktop smoke needs JSON configuration")
	}
	var once sync.Once
	app.Event.On("m4:smoke", func(e *application.CustomEvent) {
		once.Do(func() {
			message, ok := e.Data.(string)
			if !ok || message != "passed" {
				log.Print("NATIVE DESKTOP SMOKE FAILED: ", e.Data)
				app.Quit()
				return
			}
			// Exercise the production close hook, then restore the window. Sleep/wake
			// semantics are exercised without suspending the developer's machine.
			session.Sleep()
			session.Wake()
			window.Close()
			time.AfterFunc(time.Second, func() {
				hidden := !window.IsVisible() || window.IsMinimised()
				// GTK/GDK on GNOME Wayland does not report MINIMIZED back to the
				// client. The compositor hidden state is separately observed in the
				// evidence; here require the window to yield focus after close.
				if runtime.GOOS == "linux" && os.Getenv("WAYLAND_DISPLAY") != "" {
					hidden = hidden || !window.IsFocused()
				}
				if !hidden {
					log.Print("NATIVE DESKTOP SMOKE FAILED: close did not hide/minimize")
				} else {
					log.Print("NATIVE DESKTOP SMOKE PASSED")
				}
				window.UnMinimise()
				window.Show()
				app.Quit()
			})
		})
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { window.ExecJS("window.__m4Smoke = " + config + ";\n" + smokeScript) })
}
