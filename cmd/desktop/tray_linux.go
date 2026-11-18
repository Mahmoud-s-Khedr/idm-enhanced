//go:build linux

package main

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// Wails registers with this watcher. Without a host, keep a taskbar entry so
// closing the window cannot strand a running application on e.g. stock GNOME.
func trayAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var available bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&available)
	return err == nil && available
}
