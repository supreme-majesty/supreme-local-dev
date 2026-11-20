// Package assets embeds the built dashboard (sld-dashboard/dist), which the
// image build copies into gui/ before compiling.
package assets

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed gui
var assetsFS embed.FS

// GetGuiFS returns a file system for the GUI assets.
func GetGuiFS() (http.FileSystem, error) {
	sub, err := fs.Sub(assetsFS, "gui")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
