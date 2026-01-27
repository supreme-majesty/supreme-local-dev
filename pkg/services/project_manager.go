package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
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
	Type       string `json:"type"` // laravel, react, nextjs, nodejs
	Name       string `json:"name"`
	Directory  string `json:"directory"`  // Optional: Specific parent directory
	Repository string `json:"repository"` // Optional: Git URL for custom type
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
	{ID: "rider", Name: "Rider", Bin: "rider", Icon: "terminal"},
	{ID: "sublime", Name: "Sublime Text", Bin: "subl", Icon: "sublime"},
	{ID: "atom", Name: "Atom", Bin: "atom", Icon: "atom"},
	{ID: "antigravity", Name: "Antigravity", Bin: "antigravity", Icon: "terminal"},
}

// DetectEditors scans path for available editors
func (pm *ProjectManager) DetectEditors() []Editor {
	var available []Editor
	seenBins := make(map[string]bool)

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

	// 1. Check supported editors list first (curated)
	for _, ed := range supportedEditors {
		found := false

		// Check PATH
		if path, err := exec.LookPath(ed.Bin); err == nil && path != "" {
			ed.Bin = path

			// Resolve symlinks to avoid duplicates
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				if seenBins[resolved] {
					continue
				}
				seenBins[resolved] = true
			} else {
				if seenBins[path] {
					continue
				}
				seenBins[path] = true
			}

			available = append(available, ed)
			found = true
		}

		// Check explicit paths if not found
		if !found {
			for _, searchPath := range extraPaths {
				fullPath := filepath.Join(searchPath, ed.Bin)
				if _, err := os.Stat(fullPath); err == nil {
					ed.Bin = fullPath

					// Resolve symlinks
					if resolved, err := filepath.EvalSymlinks(fullPath); err == nil {
						if seenBins[resolved] {
							continue
						}
						seenBins[resolved] = true
					} else {
						if seenBins[fullPath] {
							continue
						}
						seenBins[fullPath] = true
					}

					available = append(available, ed)
					found = true
					break
				}
			}
		}
	}

	// 2. Scan desktop files for other editors (Linux only)
	if runtime.GOOS == "linux" {
		desktopEditors := pm.scanDesktopFiles()
		for _, ed := range desktopEditors {
			// Also exclude non-web-dev editors if found via desktop file
			nameLower := strings.ToLower(ed.Name)
			idLower := strings.ToLower(ed.ID)

			// Filters for CLI tools and non-web IDEs
			if idLower == "text-editor" || strings.Contains(nameLower, "text editor") {
				continue // Gnome Text Editor doesn't support opening directories directly as projects
			}
			if idLower == "nano" || idLower == "vim" || idLower == "nvim" || idLower == "emacs" {
				continue
			}
			if idLower == "android-studio" || strings.Contains(nameLower, "android studio") {
				continue
			}
			if idLower == "arduino-ide" || strings.Contains(nameLower, "arduino") {
				continue
			}

			// Filter out URL handlers which are usually duplicates
			if strings.Contains(nameLower, "url handler") || strings.Contains(nameLower, "new window") {
				continue
			}

			// Check against seen binaries
			path := ed.Bin
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				path = resolved
			}

			if !seenBins[path] {
				available = append(available, ed)
				seenBins[path] = true
			}
		}
	}

	return available
}

// scanDesktopFiles looks for editor .desktop files in standard locations
func (pm *ProjectManager) scanDesktopFiles() []Editor {
	dirs := []string{
		"/usr/share/applications",
		"/var/lib/snapd/desktop/applications",
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "applications"))
	}

	var found []Editor

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".desktop") {
				path := filepath.Join(dir, entry.Name())
				if ed, ok := pm.parseDesktopFile(path); ok {
					found = append(found, ed)
				}
			}
		}
	}
	return found
}

// parseDesktopFile attempts to read a .desktop file and identify if it's an editor
func (pm *ProjectManager) parseDesktopFile(path string) (Editor, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Editor{}, false
	}

	lines := strings.Split(string(content), "\n")

	var name, execCmd, icon, categories string
	var isApp bool

	inDesktopEntry := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			if line == "[Desktop Entry]" {
				inDesktopEntry = true
			} else {
				inDesktopEntry = false
			}
			continue
		}

		if !inDesktopEntry {
			continue
		}

		if strings.HasPrefix(line, "Type=") {
			if line == "Type=Application" {
				isApp = true
			}
		} else if strings.HasPrefix(line, "Name=") {
			name = strings.TrimPrefix(line, "Name=")
		} else if strings.HasPrefix(line, "Exec=") {
			execCmd = strings.TrimPrefix(line, "Exec=")
		} else if strings.HasPrefix(line, "Icon=") {
			icon = strings.TrimPrefix(line, "Icon=")
		} else if strings.HasPrefix(line, "Categories=") {
			categories = strings.TrimPrefix(line, "Categories=")
		}
	}

	// Validations
	if !isApp {
		return Editor{}, false
	}

	// Must be an editor/IDE
	isEditor := strings.Contains(categories, "TextEditor") ||
		strings.Contains(categories, "IDE") ||
		strings.Contains(categories, "Development")

	// Filter out false positives if just "Development"
	if strings.Contains(categories, "Development") && !strings.Contains(categories, "TextEditor") && !strings.Contains(categories, "IDE") {
		// Example: "Qt Designer" is Development but not an IDE/Editor usually desired
		// For now, let's include "Development;IDE" or "TextEditor"
		if !strings.Contains(categories, "IDE") {
			isEditor = false
		}
	}
	// Always allow explicit TextEditor
	if strings.Contains(categories, "TextEditor") {
		isEditor = true
	}

	if !isEditor || execCmd == "" || name == "" {
		return Editor{}, false
	}

	// Clean Exec command (remove placeholders like %F, %U, and arguments)
	// Simple heuristic: Take first token.
	// NOTE: Paths with spaces in quotes are tricky, but rare in standardized .desktop Execs
	// Usually: Exec=/path/to/bin %F
	fields := strings.Fields(execCmd)
	if len(fields) > 0 {
		execCmd = fields[0]
	}

	// Remove quotes if present
	execCmd = strings.Trim(execCmd, "\"")

	// Must verify executable exists
	if _, err := exec.LookPath(execCmd); err != nil {
		// Try absolute path if it is one
		if filepath.IsAbs(execCmd) {
			if _, err := os.Stat(execCmd); err != nil {
				return Editor{}, false
			}
		} else {
			return Editor{}, false
		}
	}

	// Generate ID from name
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	return Editor{
		ID:   id,
		Name: name,
		Bin:  execCmd,
		Icon: icon, // Frontend might not support random icons, but we pass it
	}, true
}

// ListDirectories returns subdirectories in the given path
func (pm *ProjectManager) ListDirectories(path string) ([]string, error) {
	if path == "" {
		path = pm.BaseDir
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directories in %s: %w", path, err)
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
	available := pm.DetectEditors()
	for _, ed := range available {
		if ed.ID == editorID {
			bin = ed.Bin
			break
		}
	}

	// Fallback to supported list if not detected (weird, but safe)
	if bin == "" {
		for _, ed := range supportedEditors {
			if ed.ID == editorID {
				bin = ed.Bin
				break
			}
		}
	}

	if bin == "" {
		return fmt.Errorf("unknown editor: %s", editorID)
	}

	// Verify path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	var cmd *exec.Cmd
	targetUser := os.Getenv("SUDO_USER")

	fmt.Printf("[DEBUG] Launching editor. Path: %s, EditorID: %s, Bin: %s\n", path, editorID, bin)

	// If SUDO_USER is empty (running as pure systemd service), try to detect user from file ownership
	// If SUDO_USER is empty (running as pure systemd service), try to detect user from file ownership
	if targetUser == "" && os.Geteuid() == 0 {
		uidInt, _, err := getPathOwner(path)
		if err == nil {
			uid := strconv.Itoa(uidInt)
			if u, err := user.LookupId(uid); err == nil {
				targetUser = u.Username
				fmt.Printf("[DEBUG] Detected owner of %s is %s (uid %s)\n", path, targetUser, uid)
			} else {
				fmt.Printf("[DEBUG] Failed to lookup user for uid %s: %v\n", uid, err)
			}
		} else {
			fmt.Printf("[DEBUG] Failed to get path owner %s: %v\n", path, err)
		}
	} else {
		fmt.Printf("[DEBUG] SUDO_USER present: %s\n", targetUser)
	}

	// If running as root and we have a target user, drop privileges and set display
	// This is primarily for Linux systemd services
	if os.Geteuid() == 0 && targetUser != "" && runtime.GOOS == "linux" {
		fmt.Printf("[DEBUG] Preparing to launch editor as user: %s\n", targetUser)

		// Get UID for target user
		var uid string
		if u, err := user.Lookup(targetUser); err == nil {
			uid = u.Uid
		} else {
			// Fallback if lookup fails
			out, _ := exec.Command("id", "-u", targetUser).Output()
			uid = strings.TrimSpace(string(out))
			fmt.Printf("[DEBUG] User lookup fallback for %s: %s\n", targetUser, uid)
		}

		runtimeDir := fmt.Sprintf("/run/user/%s", uid)

		// Dynamically discover GUI environment variables from user's running processes
		guiEnv := map[string]string{
			"DISPLAY":         ":0", // Default fallback
			"XDG_RUNTIME_DIR": runtimeDir,
		}

		// Attempt to scrape environment from recent user processes
		// We look for processes owned by the user
		// Use full path for pgrep as it might not be in PATH for systemd service
		if pids, err := exec.Command("/usr/bin/pgrep", "-u", targetUser).Output(); err == nil {
			pidList := strings.Fields(string(pids))
			// Check recent processes first (reverse order)
			for i := len(pidList) - 1; i >= 0; i-- {
				pid := pidList[i]
				envPath := fmt.Sprintf("/proc/%s/environ", pid)
				content, err := os.ReadFile(envPath)
				if err != nil {
					continue // Skip silently, many processes won't be readable
				}

				// Parse null-terminated environment
				envData := string(content)

				// Critical: We should prefer a process that has BOTH DISPLAY and XAUTHORITY.
				if strings.Contains(envData, "DISPLAY=") {
					parts := strings.Split(envData, "\x00")

					// Temp map for this process
					processEnv := make(map[string]string)

					for _, p := range parts {
						if strings.HasPrefix(p, "DISPLAY=") ||
							strings.HasPrefix(p, "WAYLAND_DISPLAY=") ||
							strings.HasPrefix(p, "XAUTHORITY=") ||
							strings.HasPrefix(p, "DBUS_SESSION_BUS_ADDRESS=") {
							kv := strings.SplitN(p, "=", 2)
							if len(kv) == 2 {
								processEnv[kv[0]] = kv[1]
							}
						}
					}

					// Update guiEnv with what we found
					for k, v := range processEnv {
						guiEnv[k] = v
					}

					// If we found XAUTHORITY, this is the golden ticket. Stop searching.
					if _, ok := processEnv["XAUTHORITY"]; ok {
						fmt.Printf("[DEBUG] Inherited valid GUI env (with XAUTHORITY) from PID %s: %v\n", pid, guiEnv)
						break
					}
				}
			}
		} else {
			fmt.Printf("[DEBUG] pgrep failed or no processes found: %v\n", err)
		}

		// Fallback: If XAUTHORITY wasn't found, try common locations for Wayland/XWayland sessions
		if _, hasXauth := guiEnv["XAUTHORITY"]; !hasXauth {
			fmt.Printf("[DEBUG] XAUTHORITY not found via process scan, trying fallback paths...\n")

			// Common XAUTHORITY locations to try
			xauthPaths := []string{
				filepath.Join(runtimeDir, ".Xauthority"),
				filepath.Join("/home", targetUser, ".Xauthority"),
			}

			// Also check for mutter/XWayland auth files (GNOME/Wayland)
			if entries, err := os.ReadDir(runtimeDir); err == nil {
				for _, entry := range entries {
					name := entry.Name()
					// Look for .mutter-Xwaylandauth.* files (GNOME on Wayland)
					if strings.HasPrefix(name, ".mutter-Xwaylandauth.") {
						xauthPaths = append([]string{filepath.Join(runtimeDir, name)}, xauthPaths...)
					}
					// Also check for gdm Xauthority
					if name == "gdm" {
						gdmAuth := filepath.Join(runtimeDir, name, "Xauthority")
						xauthPaths = append([]string{gdmAuth}, xauthPaths...)
					}
				}
			}

			// Try each path until we find a valid one
			for _, xaPath := range xauthPaths {
				if info, err := os.Stat(xaPath); err == nil && !info.IsDir() {
					guiEnv["XAUTHORITY"] = xaPath
					fmt.Printf("[DEBUG] Found XAUTHORITY via fallback: %s\n", xaPath)
					break
				}
			}

			if _, hasXauth := guiEnv["XAUTHORITY"]; !hasXauth {
				fmt.Printf("[WARN] Could not find XAUTHORITY - X11 apps may fail to display\n")
			}
		}

		// Construct environment arguments
		var envVars []string
		for k, v := range guiEnv {
			envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
		}

		// Wrap command to run in background with nohup style detachment
		debugLog := fmt.Sprintf("/tmp/sld-editor-%s.log", targetUser)
		// Use setsid to fully detach the process from the controlling terminal
		wrappedCmd := fmt.Sprintf("nohup %s %s > %s 2>&1 &", bin, path, debugLog)

		cmdArgs := []string{
			"-u", targetUser,
			"env",
		}
		cmdArgs = append(cmdArgs, envVars...)
		cmdArgs = append(cmdArgs, "/bin/sh", "-c", wrappedCmd)

		fmt.Printf("[DEBUG] Executing: sudo %v\n", cmdArgs)
		cmd = exec.Command("sudo", cmdArgs...)
	} else {
		// Non-root or non-Linux execution
		fmt.Printf("[DEBUG] Executing direct: %s %s\n", bin, path)
		cmd = exec.Command(bin, path)
	}

	// Use Start() instead of CombinedOutput() to not block waiting for editor to close
	if err := cmd.Start(); err != nil {
		fmt.Printf("[ERROR] Editor launch failed: %v\n", err)
		return fmt.Errorf("editor launch failed: %w", err)
	}

	// Don't wait for the process to finish - editors are long-running
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Printf("[DEBUG] Editor process ended with: %v (this is normal if user closed it)\n", err)
		}
	}()

	fmt.Printf("[DEBUG] Editor launched successfully. Check /tmp/sld-editor-*.log\n")

	return nil
}

// Template represents a project template
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"` // e.g. "wordpress", "html", "git"
}

// GetTemplates returns available project templates
func (pm *ProjectManager) GetTemplates() []Template {
	return []Template{
		{ID: "laravel", Name: "Laravel", Description: "Modern PHP framework for web artisans", Icon: "laravel"},
		{ID: "wordpress", Name: "WordPress", Description: "The world's most popular CMS", Icon: "wordpress"},
		{ID: "react", Name: "React", Description: "A JavaScript library for building user interfaces", Icon: "react"},
		{ID: "vue", Name: "Vue.js", Description: "The Progressive JavaScript Framework", Icon: "vue"},
		{ID: "nextjs", Name: "Next.js", Description: "The React Framework for the Web", Icon: "nextjs"},
		{ID: "nodejs", Name: "Node.js", Description: "Basic Node.js project", Icon: "nodejs"},
		{ID: "static", Name: "Static HTML", Description: "Simple HTML/CSS/JS project", Icon: "html"},
		{ID: "custom", Name: "Custom (Git)", Description: "Clone from a Git repository", Icon: "git"},
	}
}

// CreateProject creates a new project using npx or composer
func (pm *ProjectManager) CreateProject(options ProjectOptions) error {
	// Sanitize name
	if strings.Contains(options.Name, "/") || strings.Contains(options.Name, "\\") || strings.Contains(options.Name, " ") {
		return fmt.Errorf("invalid project name: must be alphanumeric and no spaces")
	}

	// Determine base directory
	base := pm.BaseDir
	if options.Directory != "" {
		base = options.Directory
	}

	// Determine intended owner from parent
	uid, gid, _ := getPathOwner(base)

	// Ensure base directory exists (and set ownership)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		if err := os.MkdirAll(base, 0755); err != nil {
			return fmt.Errorf("failed to create base directory %s: %w", base, err)
		}
		// Set ownership of the new directory to match parent
		if uid != 0 {
			os.Chown(base, int(uid), int(gid))
		}
	}

	targetDir := filepath.Join(base, options.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("directory already exists: %s", targetDir)
	}

	var shell string = "/bin/bash"
	var cleanEnv []string

	if uid != 0 {
		u, err := user.LookupId(strconv.Itoa(int(uid)))
		if err == nil {
			// Add Herd Lite paths and standard paths
			pathStr := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
			pathStr += ":" + filepath.Join(u.HomeDir, ".local/bin")
			pathStr += ":" + filepath.Join(u.HomeDir, ".composer/vendor/bin")
			pathStr += ":" + filepath.Join(u.HomeDir, ".config/herd-lite/bin")

			cleanEnv = []string{
				"HOME=" + u.HomeDir,
				"USER=" + u.Username,
				"LOGNAME=" + u.Username,
				"PATH=" + pathStr,
				"SHELL=/bin/bash",
				"TERM=xterm-256color",
				"LANG=en_US.UTF-8",
			}

			// Composer settings
			if options.Type == "laravel" {
				composerHome := filepath.Join(u.HomeDir, ".config/composer")
				if _, err := os.Stat(composerHome); os.IsNotExist(err) {
					composerHome = filepath.Join(u.HomeDir, ".composer")
				}
				cleanEnv = append(cleanEnv, "COMPOSER_HOME="+composerHome)
				cleanEnv = append(cleanEnv, "COMPOSER_ALLOW_SUPERUSER=1")
			}
		}
	}

	// Execute via bash wrapper
	var cmdStr string
	switch options.Type {
	case "laravel":
		// Prefer composer explicitly with --no-cache to avoid corruption issues
		// We use bash to resolve 'composer' from the injected PATH
		cmdStr = fmt.Sprintf("composer create-project laravel/laravel %s --prefer-dist --no-cache", options.Name)
	case "wordpress":
		// Download latest wordpress, unzip, move contents to targetDir
		// We'll use a sequence of commands
		cmdStr = fmt.Sprintf("mkdir %s && curl -L https://wordpress.org/latest.tar.gz | tar xz -C %s --strip-components=1", options.Name, options.Name)
	case "react":
		cmdStr = fmt.Sprintf("npx -y create-vite@latest %s --template react", options.Name)
	case "vue":
		cmdStr = fmt.Sprintf("npx -y create-vite@latest %s --template vue", options.Name)
	case "nextjs":
		cmdStr = fmt.Sprintf("npx -y create-next-app@latest %s --ts --tailwind --eslint --app --no-src-dir --import-alias @/* --use-npm", options.Name)
	case "nodejs":
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return err
		}
		if uid != 0 {
			os.Chown(targetDir, int(uid), int(gid))
		}
		cmdStr = "npm init -y"
	case "static":
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return err
		}
		if uid != 0 {
			os.Chown(targetDir, int(uid), int(gid))
		}
		// Create a basic index.html
		indexPath := filepath.Join(targetDir, "index.html")
		content := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f0f2f5; }
        .card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); text-align: center; }
        h1 { margin: 0 0 1rem; color: #333; }
        p { color: #666; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Welcome to %s</h1>
        <p>Your static site is ready!</p>
    </div>
</body>
</html>`, options.Name, options.Name)
		os.WriteFile(indexPath, []byte(content), 0644)
		if uid != 0 {
			os.Chown(indexPath, int(uid), int(gid))
		}
		cmdStr = "echo 'Static site created'" // Dummy command to satisfy execution flow
	case "custom":
		if options.Repository == "" {
			return fmt.Errorf("repository URL is required for custom projects")
		}
		cmdStr = fmt.Sprintf("git clone %s %s", options.Repository, options.Name)
	default:
		return fmt.Errorf("unsupported project type: %s", options.Type)
	}

	// Execute via bash wrapper
	var cmd *exec.Cmd
	if options.Type == "nodejs" {
		cmd = exec.Command(shell, "-c", "cd "+options.Name+" && "+cmdStr)
	} else {
		cmd = exec.Command(shell, "-c", cmdStr)
	}

	cmd.Dir = base
	prepareCommand(cmd, int(uid), int(gid), cleanEnv)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("project creation failed: %s Output: %s", err, string(output))
	}

	// Post-Creation Steps (Laravel NPM)
	if options.Type == "laravel" {
		// Run npm install && npm run build
		npmCmd := exec.Command(shell, "-c", "npm install && npm run build")
		npmCmd.Dir = targetDir

		prepareCommand(npmCmd, int(uid), int(gid), cleanEnv)

		npmOutput, npmErr := npmCmd.CombinedOutput()
		if npmErr != nil {
			fmt.Printf("[WARN] npm install/build failed: %s Output: %s\n", npmErr, string(npmOutput))
		}

		// Automate Database and Permissions Setup
		fmt.Printf("[INFO] Performing post-creation setup for Laravel project...\n")

		// Determine www-data GID
		var wwwDataGid int
		if group, err := user.LookupGroup("www-data"); err == nil {
			if gid, err := strconv.Atoi(group.Gid); err == nil {
				wwwDataGid = gid
			}
		}

		// 1. Create SQLite database if it doesn't exist
		dbPath := filepath.Join(targetDir, "database", "database.sqlite")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			if f, err := os.Create(dbPath); err == nil {
				f.Close()
				if uid != 0 {
					os.Chown(dbPath, int(uid), int(gid))
				}
				os.Chmod(dbPath, 0664)
				fmt.Printf("[INFO] Created database.sqlite\n")
			} else {
				fmt.Printf("[WARN] Failed to create database.sqlite: %v\n", err)
			}
		}

		// 2. Fix Permissions (storage, bootstrap/cache, database)
		dirsToChmod := []string{
			filepath.Join(targetDir, "storage"),
			filepath.Join(targetDir, "bootstrap", "cache"),
			filepath.Join(targetDir, "database"),
		}

		for _, dir := range dirsToChmod {
			// Recursive Walk
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err == nil {
					// Change Group to www-data if found, keeping Owner as user (uid)
					if wwwDataGid != 0 && uid != 0 {
						os.Chown(path, int(uid), wwwDataGid)
					}
					// Allow Group Write (775)
					os.Chmod(path, 0775)
				}
				return nil
			})
		}

		// Also fix database file specifically
		if wwwDataGid != 0 && uid != 0 {
			os.Chown(dbPath, int(uid), wwwDataGid)
		}
		os.Chmod(dbPath, 0664) // rw-rw-r--

		// 3. Run Migrations
		migrateCmd := exec.Command(shell, "-c", "php artisan migrate --force")
		migrateCmd.Dir = targetDir
		prepareCommand(migrateCmd, int(uid), int(gid), cleanEnv)
		if out, err := migrateCmd.CombinedOutput(); err != nil {
			fmt.Printf("[WARN] Migration failed: %v Output: %s\n", err, string(out))
		} else {
			fmt.Printf("[INFO] Migrations ran successfully\n")
		}
	}

	return nil
}

// CloneProject creates a "Ghost" clone of a project for experimentation.
// It copies the project files (excluding heavy dirs) and optionally clones its database.
func (pm *ProjectManager) CloneProject(sourcePath, targetName string, cloneDB bool, dbService interface {
	CloneDatabase(source, target string) error
}) (string, error) {
	// 1. Validate source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "", fmt.Errorf("source project not found: %s", sourcePath)
	}

	// 2. Determine target path (same parent as source, with -ghost suffix)
	sourceDir := filepath.Dir(sourcePath)
	if targetName == "" {
		targetName = filepath.Base(sourcePath) + "-ghost"
	}
	targetPath := filepath.Join(sourceDir, targetName)

	// Check target doesn't already exist
	if _, err := os.Stat(targetPath); err == nil {
		return "", fmt.Errorf("target path already exists: %s", targetPath)
	}

	// 3. Copy files using rsync for speed (excluding heavy directories)
	// Exclude: node_modules, vendor, .git, storage/logs, storage/framework/cache
	rsyncArgs := []string{
		"-a", "--progress",
		"--exclude", "node_modules",
		"--exclude", "vendor",
		"--exclude", ".git",
		"--exclude", "storage/logs/*",
		"--exclude", "storage/framework/cache/*",
		"--exclude", "storage/framework/sessions/*",
		"--exclude", "storage/framework/views/*",
		sourcePath + "/",
		targetPath,
	}

	cmd := exec.Command("rsync", rsyncArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to copy project: %s", string(output))
	}

	fmt.Printf("[GHOST MODE] Copied project to %s\n", targetPath)

	// 4. If Laravel project and cloneDB requested, clone the database
	if cloneDB {
		envPath := filepath.Join(targetPath, ".env")
		if _, err := os.Stat(envPath); err == nil {
			// Read .env to get DB name
			envContent, err := os.ReadFile(envPath)
			if err == nil {
				lines := strings.Split(string(envContent), "\n")
				var sourceDBName string
				for _, line := range lines {
					if strings.HasPrefix(line, "DB_DATABASE=") {
						sourceDBName = strings.TrimPrefix(line, "DB_DATABASE=")
						sourceDBName = strings.TrimSpace(sourceDBName)
						break
					}
				}

				if sourceDBName != "" && dbService != nil {
					targetDBName := targetName + "_db"
					// Replace dashes with underscores for DB name validity
					targetDBName = strings.ReplaceAll(targetDBName, "-", "_")

					fmt.Printf("[GHOST MODE] Cloning database %s -> %s\n", sourceDBName, targetDBName)
					if err := dbService.CloneDatabase(sourceDBName, targetDBName); err != nil {
						fmt.Printf("[GHOST MODE] Warning: DB clone failed: %v\n", err)
					} else {
						// Update .env in target to point to new DB
						newEnvContent := strings.Replace(string(envContent),
							"DB_DATABASE="+sourceDBName,
							"DB_DATABASE="+targetDBName, 1)
						os.WriteFile(envPath, []byte(newEnvContent), 0644)
						fmt.Printf("[GHOST MODE] Updated .env with new database name\n")
					}
				}
			}
		}
	}

	// 5. Update APP_URL in .env if it exists
	envPath := filepath.Join(targetPath, ".env")
	if envContent, err := os.ReadFile(envPath); err == nil {
		sourceName := filepath.Base(sourcePath)
		newEnvContent := strings.Replace(string(envContent),
			sourceName+".test",
			targetName+".test", -1)
		os.WriteFile(envPath, []byte(newEnvContent), 0644)
	}

	return targetPath, nil
}

// DiscardGhost removes a ghost clone and its database.
func (pm *ProjectManager) DiscardGhost(path string, dbName string, dbService interface {
	DeleteDatabase(name string) error
}) error {
	// 1. Delete Database if provided
	if dbName != "" && dbService != nil {
		fmt.Printf("[GHOST MODE] Deleting ghost database: %s\n", dbName)
		if err := dbService.DeleteDatabase(dbName); err != nil {
			fmt.Printf("[GHOST MODE] Warning: Failed to delete ghost database: %v\n", err)
		}
	}

	// 2. Delete Project Directory
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("[GHOST MODE] Deleting ghost project directory: %s\n", path)
		return os.RemoveAll(path)
	}

	return nil
}

// PackageJSON represents package.json structure for engine parsing
type PackageJSON struct {
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
}

// ScanNodeRequirement reads package.json to find node version requirement
func (pm *ProjectManager) ScanNodeRequirement(projectPath string) (string, error) {
	pkgPath := filepath.Join(projectPath, "package.json")
	if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
		return "", nil // No package.json, no requirement
	}

	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", err
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", nil // Ignore invalid json
	}

	return pkg.Engines.Node, nil
}
