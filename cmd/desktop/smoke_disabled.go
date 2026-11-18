//go:build !desktop_smoke

package main

import (
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The native UI driver is compiled only into explicitly tagged test binaries.
func configureSmoke(_ *application.App, _ *application.WebviewWindow, _ *manager.Session) {}
