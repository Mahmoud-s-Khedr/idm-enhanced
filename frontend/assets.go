// Package frontend embeds the built desktop assets; run task build before native compilation.
package frontend

import "embed"

//go:embed all:dist
var Assets embed.FS
