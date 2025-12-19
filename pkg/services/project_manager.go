package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ProjectManager struct {
	BaseDir string // Default directory for new projects (e.g. ~/Developments)
}

func NewProjectManager(baseDir string) *ProjectManager {
	return &ProjectManager{
		BaseDir: baseDir,
	}
}

// Editor represents a text editor or IDE
type Editor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Bin  string `json:"bin"`
	Icon string `json:"icon"` // Optional icon name for frontend
}

// ProjectOptions options for creating a project
type ProjectOptions struct {
	Type      string `json:"type"` // laravel, react, nextjs, nodejs
	Name      string `json:"name"`
	Directory string `json:"directory"` // Optional: Specific parent directory
}

// Editors supported for detection
var supportedEditors = []Editor{
	{ID: "vscode", Name: "VS Code", Bin: "code", Icon: "vscode"},
	{ID: "cursor", Name: "Cursor", Bin: "cursor", Icon: "cursor"},
	{ID: "vscodium", Name: "VSCodium", Bin: "codium", Icon: "vscode"},
	{ID: "zed", Name: "Zed", Bin: "zed", Icon: "terminal"},
	{ID: "phpstorm", Name: "PhpStorm", Bin: "phpstorm", Icon: "phpstorm"},
	{ID: "webstorm", Name: "WebStorm", Bin: "webstorm", Icon: "terminal"},
	{ID: "intellij", Name: "IntelliJ IDEA", Bin: "idea", Icon: "terminal"},
	{ID: "intellij-ult", Name: "IntelliJ IDEA (Ultimate)", Bin: "intellij-idea-ultimate", Icon: "terminal"},
	{ID: "intellij-ce", Name: "IntelliJ IDEA (CE)", Bin: "intellij-idea-community", Icon: "terminal"},
	{ID: "goland", Name: "GoLand", Bin: "goland", Icon: "terminal"},
	{ID: "pycharm", Name: "PyCharm", Bin: "charm", Icon: "terminal"},
	{ID: "pycharm-pro", Name: "PyCharm (Pro)", Bin: "pycharm-professional", Icon: "terminal"},
	{ID: "pycharm-ce", Name: "PyCharm (CE)", Bin: "pycharm-community", Icon: "terminal"},
	{ID: "android-studio", Name: "Android Studio", Bin: "android-studio", Icon: "terminal"},
	{ID: "clion", Name: "CLion", Bin: "clion", Icon: "terminal"},
	{ID: "rider", Name: "Rider", Bin: "rider", Icon: "terminal"},
	{ID: "sublime", Name: "Sublime Text", Bin: "subl", Icon: "sublime"},
	{ID: "atom", Name: "Atom", Bin: "atom", Icon: "atom"},
	{ID: "nvim", Name: "Neovim", Bin: "nvim", Icon: "terminal"},
	{ID: "vim", Name: "Vim", Bin: "vim", Icon: "terminal"},
	{ID: "nano", Name: "Nano", Bin: "nano", Icon: "terminal"},
	{ID: "emacs", Name: "Emacs", Bin: "emacs", Icon: "terminal"},
	{ID: "antigravity", Name: "Antigravity", Bin: "antigravity", Icon: "terminal"},
}

// DetectEditors scans path for available editors
func (pm *ProjectManager) DetectEditors() []Editor {
	var available []Editor
	// Common paths to check beyond just PATH
	extraPaths := []string{
		"/snap/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/opt/bin",
	}

	// Add ~/.local/bin
	if home, err := os.UserHomeDir(); err == nil {
		extraPaths = append(extraPaths, filepath.Join(home, ".local", "bin"))
		extraPaths = append(extraPaths, filepath.Join(home, "Applications")) // Common for AppImages
	}

	for _, ed := range supportedEditors {
		found := false

		// 1. Check PATH
		if path, err := exec.LookPath(ed.Bin); err == nil && path != "" {
			// Update binary path to absolute if found
			ed.Bin = path
			available = append(available, ed)
			found = true
		}

		// 2. Check explicit paths if not found
		if !found {
			for _, searchPath := range extraPaths {
				fullPath := filepath.Join(searchPath, ed.Bin)
				if _, err := os.Stat(fullPath); err == nil {
					ed.Bin = fullPath
					available = append(available, ed)
					found = true
					break
				}
			}
		}

		// 3. MacOS specific checks (optional, keeping placeholder)
		if !found && runtime.GOOS == "darwin" {
			// ...
		}
	}
	return available
}

// ListDirectories returns subdirectories in the given path
func (pm *ProjectManager) ListDirectories(path string) ([]string, error) {
	if path == "" {
		path = pm.BaseDir
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var dirs []string
	// Add parent directory option if technically possible, but let's stick to children for now
	// Ideally we want full navigation.

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs, nil
}

// OpenInEditor opens the path in the specified editor
func (pm *ProjectManager) OpenInEditor(path string, editorID string) error {
	var bin string

	// Find the binary for the requested editor
	for _, ed := range supportedEditors {
		if ed.ID == editorID {
			bin = ed.Bin
			break
		}
	}

	if bin == "" {
		return fmt.Errorf("unknown editor: %s", editorID)
	}

	// Verify path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	cmd := exec.Command(bin, path)
	return cmd.Start() // Non-blocking
}

// CreateProject creates a new project using npx or composer
func (pm *ProjectManager) CreateProject(options ProjectOptions) error {
	// Sanitize name
	if strings.Contains(options.Name, "/") || strings.Contains(options.Name, "\\") || strings.Contains(options.Name, " ") {
		return fmt.Errorf("invalid project name: must be alphanumeric and no spaces")
	}

	// Determine target directory
	base := pm.BaseDir
	if options.Directory != "" {
		base = options.Directory
	}

	targetDir := filepath.Join(base, options.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("directory already exists: %s", targetDir)
	}

	var cmd *exec.Cmd

	switch options.Type {
	case "laravel":
		// composer create-project laravel/laravel:^10.0 example-app
		// We'll default to latest stable
		cmd = exec.Command("composer", "create-project", "laravel/laravel", options.Name)
	case "react":
		// npx create-vite@latest my-vue-app --template react
		cmd = exec.Command("npx", "-y", "create-vite@latest", options.Name, "--template", "react")
	case "vue":
		cmd = exec.Command("npx", "-y", "create-vite@latest", options.Name, "--template", "vue")
	case "nextjs":
		// npx create-next-app@latest
		// This is interactive by default. We need to pass flags to make it non-interactive.
		// --use-npm, --ts, --tailwind, --eslint, --app, --src-dir, --import-alias "@/*"
		cmd = exec.Command("npx", "-y", "create-next-app@latest", options.Name,
			"--ts", "--tailwind", "--eslint", "--app", "--no-src-dir", "--import-alias", "@/*", "--use-npm")
	case "nodejs":
		// Basic npm init
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return err
		}
		cmd = exec.Command("npm", "init", "-y")
		cmd.Dir = targetDir // Run INSIDE the dir
	default:
		return fmt.Errorf("unsupported project type: %s", options.Type)
	}

	// If cmd.Dir wasn't set (because the command creates the dir), run in BaseDir
	if cmd.Dir == "" {
		cmd.Dir = base
	}

	// Capture output?
	// For a real app, we'd want to stream this to a websocket.
	// For now, we'll just run it and return error if it fails.
	// But `create-next-app` etc might take a while.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("project creation failed: %s\nOutput: %s", err, string(output))
	}

	return nil
}
