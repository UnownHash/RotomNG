//go:build no_embed_ui

package rotom_ui

import "embed"

func GetUIFS() *embed.FS {
	return nil
}
