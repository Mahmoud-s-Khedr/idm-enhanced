package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type notificationFixture struct {
	startup, shutdown, delivery, permission error
	allowed                                 bool
	sent, stopped, requested                int
}

func (n *notificationFixture) ServiceStartup(context.Context, application.ServiceOptions) error {
	return n.startup
}
func (n *notificationFixture) ServiceShutdown() error { n.stopped++; return n.shutdown }
func (n *notificationFixture) CheckNotificationAuthorization() (bool, error) {
	return n.allowed, n.permission
}
func (n *notificationFixture) RequestNotificationAuthorization() (bool, error) {
	n.requested++
	return n.allowed, n.permission
}
func (n *notificationFixture) SendNotification(options notifications.NotificationOptions) error {
	if options.Title != "Download complete" {
		return errors.New("wrong title")
	}
	n.sent++
	return n.delivery
}
func TestOptionalNotifications(t *testing.T) {
	for _, mode := range []string{"ok", "startup", "cleanup", "denied", "permission", "delivery", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			backend := &notificationFixture{allowed: true}
			failure := errors.New("fixture failure")
			switch mode {
			case "startup":
				backend.startup = failure
			case "cleanup":
				backend.startup = failure
				backend.shutdown = failure
			case "denied":
				backend.allowed = false
			case "permission":
				backend.permission = failure
			case "delivery":
				backend.delivery = failure
			case "shutdown":
				backend.shutdown = failure
			}
			service := desktop.New("ready")
			n := &completionNotifications{service: backend, desktop: service}
			if err := n.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatal("optional notifications stopped app", err)
			}
			n.send("job")
			if backend.requested != 0 {
				t.Fatal("download completion requested permission")
			}
			if mode != "ok" && mode != "shutdown" && service.Info().Notification == "" {
				t.Fatal("failure invisible")
			}
			err := n.authorize()
			if mode == "ok" && err != nil {
				t.Fatal(err)
			}
			if err == nil && service.Info().Notification != "" {
				t.Fatal("recovered notification failure remained visible")
			}
			if err = n.ServiceShutdown(); (err != nil) != (mode == "shutdown") {
				t.Fatal(err)
			}
			if backend.stopped != 1 {
				t.Fatal("startup resources not released", backend.stopped)
			}
		})
	}
	if len(trayIcon()) == 0 {
		t.Fatal("tray icon missing")
	}
}
