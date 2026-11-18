package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"sync"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Notifications are optional: an unsigned macOS build or a Linux desktop without
// a notification daemon must still be able to download and report completion.
type notificationBackend interface {
	ServiceStartup(context.Context, application.ServiceOptions) error
	ServiceShutdown() error
	CheckNotificationAuthorization() (bool, error)
	RequestNotificationAuthorization() (bool, error)
	SendNotification(notifications.NotificationOptions) error
}
type completionNotifications struct {
	mu      sync.Mutex
	ready   bool
	service notificationBackend
	desktop *desktop.Service
}

func (n *completionNotifications) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	err := n.service.ServiceStartup(ctx, options)
	n.mu.Lock()
	n.ready = err == nil
	n.mu.Unlock()
	if err != nil {
		n.desktop.NotificationFailure()
		if cleanupErr := n.service.ServiceShutdown(); cleanupErr != nil {
			log.Print("Notification startup cleanup failed")
		}
	}
	return nil
}
func (n *completionNotifications) ServiceShutdown() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ready {
		n.ready = false
		return n.service.ServiceShutdown()
	}
	return nil
}
func (n *completionNotifications) send(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.ready {
		return
	}
	allowed, err := n.service.CheckNotificationAuthorization()

	if err != nil || !allowed {
		n.desktop.NotificationFailure()
		return
	}
	if err = n.service.SendNotification(notifications.NotificationOptions{ID: id, Title: "Download complete", Body: "Your verified file is ready. Open IDM Enhanced to view it."}); err != nil {
		n.desktop.NotificationFailure()
	} else {
		n.desktop.NotificationReady()
	}
}
func trayIcon() []byte {
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(icon, icon.Bounds(), &image.Uniform{C: color.RGBA{R: 24, G: 70, B: 65, A: 255}}, image.Point{}, draw.Src)
	ink := &image.Uniform{C: color.RGBA{R: 130, G: 219, B: 195, A: 255}}
	for _, r := range []image.Rectangle{image.Rect(13, 5, 19, 21), image.Rect(8, 16, 24, 21), image.Rect(10, 21, 22, 24), image.Rect(5, 27, 27, 30)} {
		draw.Draw(icon, r, ink, image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, icon); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// Authorization is an explicit settings action; download completion must not
// block shutdown on a permission prompt.
func (n *completionNotifications) authorize() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.ready {
		return errors.New("system notifications unavailable")
	}
	allowed, err := n.service.RequestNotificationAuthorization()
	if err != nil || !allowed {
		n.desktop.NotificationFailure()
		return errors.New("system notification permission unavailable")
	}
	n.desktop.NotificationReady()
	return nil
}
