package services

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
	if targetUser == "" && os.Geteuid() == 0 {
		info, err := os.Stat(path)
		if err == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				uid := strconv.Itoa(int(stat.Uid))
				if u, err := user.LookupId(uid); err == nil {
					targetUser = u.Username
					fmt.Printf("[DEBUG] Detected owner of %s is %s (uid %s)\n", path, targetUser, uid)
				} else {
					fmt.Printf("[DEBUG] Failed to lookup user for uid %s: %v\n", uid, err)
				}
			}
		} else {
			fmt.Printf("[DEBUG] Failed to stat path %s: %v\n", path, err)
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

	// Construct Command String
	var cmdStr string
	switch options.Type {
	case "laravel":
		// Prefer composer explicitly with --no-cache to avoid corruption issues
		// We use bash to resolve 'composer' from the injected PATH
		cmdStr = fmt.Sprintf("composer create-project laravel/laravel %s --prefer-dist --no-cache", options.Name)
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
	if uid != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
		cmd.Env = cleanEnv
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("project creation failed: %s Output: %s", err, string(output))
	}

	// Post-Creation Steps (Laravel NPM)
	if options.Type == "laravel" {
		// Run npm install && npm run build
		npmCmd := exec.Command(shell, "-c", "npm install && npm run build")
		npmCmd.Dir = targetDir

		if uid != 0 {
			npmCmd.SysProcAttr = &syscall.SysProcAttr{}
			npmCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}
			npmCmd.Env = cleanEnv
		}

		npmOutput, npmErr := npmCmd.CombinedOutput()
		if npmErr != nil {
			fmt.Printf("[WARN] npm install/build failed: %s Output: %s\n", npmErr, string(npmOutput))
		}
	}

	return nil
}

// Helper to find the owner of the nearest existing directory
func getPathOwner(path string) (uint32, uint32, error) {
	for {
		if info, err := os.Stat(path); err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			return stat.Uid, stat.Gid, nil
		}
		parent := filepath.Dir(path)
		if parent == path || parent == "." || parent == "/" {
			// Reach root without success, verify root exists? Root always exists.
			// If we are here, path doesn't exist.
			if parent == "/" {
				// Stat root
				if info, err := os.Stat("/"); err == nil {
					stat := info.Sys().(*syscall.Stat_t)
					return stat.Uid, stat.Gid, nil
				}
				return 0, 0, fmt.Errorf("root not accessible")
			}
		}
		path = parent
	}
}
