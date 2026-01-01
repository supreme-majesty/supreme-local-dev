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
	if os.Geteuid() == 0 && targetUser != "" {
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
		cmd = exec.Command("composer", "create-project", "laravel/laravel", options.Name)
	case "react":
		// npx create-vite@latest my-vue-app --template react
		cmd = exec.Command("npx", "-y", "create-vite@latest", options.Name, "--template", "react")
	case "vue":
		cmd = exec.Command("npx", "-y", "create-vite@latest", options.Name, "--template", "vue")
	case "nextjs":
		// npx create-next-app@latest
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

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("project creation failed: %s\nOutput: %s", err, string(output))
	}

	return nil
}
