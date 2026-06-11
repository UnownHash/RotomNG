//go:build !no_embed_ui

// Package rotom_ui provides embedded static files for the RotomNG web UI.
package rotom_ui

import (
	"embed"
)

//go:embed static
var embedFS embed.FS

// GetUIFS returns the embedded filesystem containing the UI static assets.
func GetUIFS() *embed.FS {
	return &embedFS
}
