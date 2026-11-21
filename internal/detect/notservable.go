package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// NotServable explains why the project at root cannot run on its own as a
// web app (a WordPress plugin, an Apps Script add-on, a mobile app...), or
// returns "" when nothing rules it out. Such folders have manifests that look
// like apps, but dockerizing them alone produces nothing useful.
func NotServable(root string) string {
	at := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	head := func(name string) string {
		f, err := os.Open(filepath.Join(root, name))
		if err != nil {
			return ""
		}
		defer f.Close()
		buf := make([]byte, 8192)
		n, _ := f.Read(buf)
		return string(buf[:n])
	}

	switch {
	case at("appsscript.json") || at(".clasp.json"):
		return "Google Apps Script project: it runs on Google's servers, not in a container"
	case !at("wp-config.php") && !at("wp-config-sample.php") && at("style.css") && strings.Contains(head("style.css"), "Theme Name:"):
		return "WordPress theme: dockerize the WordPress site that uses it"
	case at("manifest.json") && strings.Contains(head("manifest.json"), `"manifest_version"`):
		return "browser extension: it runs in the browser, not in a container"
	}
	if !at("wp-config.php") && !at("wp-config-sample.php") {
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".php") && strings.Contains(head(e.Name()), "Plugin Name:") {
					return "WordPress plugin: dockerize the WordPress site that loads it"
				}
			}
		}
	}
	p := &proj{root: root, pkg: readPackageJSON(root), notes: &[]string{}}
	if kind := mobileKind(p); kind != "" {
		return kind + " app: not served over HTTP"
	}
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Engines map[string]string `json:"engines"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Engines["vscode"] != "" {
			return "VS Code extension: it runs inside the editor"
		}
	}
	return ""
}
