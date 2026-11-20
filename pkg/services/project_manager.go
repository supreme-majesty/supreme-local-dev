package services

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
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
	available := make([]Editor, 0)
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

// FileInfo represents metadata about a file or directory
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// ListFiles returns a list of files and directories in the specified path
func (pm *ProjectManager) ListFiles(dirPath string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(dirPath, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort: Directories first, then alphabetical
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

// ReadFile returns the content of a file
func (pm *ProjectManager) ReadFile(filePath string) (string, error) {
	// Security: In a real app we'd check if path is within project scope
	// For SLD (local tool), we allow it.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveFile writes content to a file
func (pm *ProjectManager) SaveFile(filePath string, content string) error {
	// Security: check path scope

	// Create backup for .env files automatically?
	if filepath.Base(filePath) == ".env" {
		em := NewEnvManager()
		em.CreateBackup(filePath)
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}
