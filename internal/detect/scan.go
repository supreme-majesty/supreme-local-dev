package detect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	appruntime "github.com/supreme-majesty/supreme-local-dev/internal/runtime"
)

// FindProjects lists project roots under dir: folders that are git
// repositories, have an SLD config, or have their own compose stack. Folders
// without any of these (e.g. "laravel-builds", "react-apps") are treated as
// groups and searched up to maxDepth levels deep. A project's sub-folders are
// not searched: they belong to it (backend/, frontend/, ...).
func FindProjects(dir string, maxDepth int) []string {
	var out []string
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if isIgnored(path) {
			return
		}
		if isProjectRoot(path) {
			out = append(out, path)
			return
		}
		if depth >= maxDepth {
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)] {
				continue
			}
			walk(filepath.Join(path, name), depth+1)
		}
	}
	walk(dir, 0)
	sort.Strings(out)
	return out
}

// IgnoreFile marks a folder (and everything below it) as not an SLD project.
const IgnoreFile = ".sldignore"

func isIgnored(path string) bool {
	_, err := os.Stat(filepath.Join(path, IgnoreFile))
	return err == nil
}

func isProjectRoot(path string) bool {
	for _, marker := range []string{".git", filepath.Join(".sld", "config.yaml")} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return len(appruntime.DefaultComposeFiles(path)) > 0
}
