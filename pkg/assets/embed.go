package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed runtime
//go:embed gui
var assetsFS embed.FS

// Extract extracts the embedded runtime assets to the destination directory.
func Extract(destDir string) error {
	return fs.WalkDir(assetsFS, "runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		// path is like "runtime/router.php"
		destPath := filepath.Join(destDir, path)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Read file
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Write file
		return os.WriteFile(destPath, data, 0644)
	})
}

// ReadTemplate reads an embedded template file.
func ReadTemplate(name string) (string, error) {
	path := fmt.Sprintf("runtime/nginx/%s", name)
	data, err := assetsFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetGuiFS returns a file system for the GUI assets.
func GetGuiFS() (http.FileSystem, error) {
	sub, err := fs.Sub(assetsFS, "gui")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
