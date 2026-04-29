package util

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Runner provides a cross-platform way to execute commands with optional elevation.
type Runner struct {
	Verbose bool
}

// NewRunner creates a new command runner.
func NewRunner(verbose bool) *Runner {
	return &Runner{Verbose: verbose}
}

// Run executes a command.
func (r *Runner) Run(name string, args ...string) error {
	if r.Verbose {
		fmt.Printf("exec: %s %s\n", name, strings.Join(args, " "))
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunElevated executes a command with administrative privileges.
func (r *Runner) RunElevated(name string, args ...string) error {
	if r.Verbose {
		fmt.Printf("exec (elevated): %s %s\n", name, strings.Join(args, " "))
	}

	switch runtime.GOOS {
	case "linux":
		// On Linux, use sudo. In a pro version, we'd check for pkexec.
		fullArgs := append([]string{name}, args...)
		return exec.Command("sudo", fullArgs...).Run()

	case "darwin":
		// On macOS, use sudo or osascript for a GUI prompt if needed.
		// For CLI installers, sudo is standard.
		fullArgs := append([]string{name}, args...)
		return exec.Command("sudo", fullArgs...).Run()

	case "windows":
		// On Windows, use PowerShell Start-Process with -Verb RunAs
		command := fmt.Sprintf("%s %s", name, strings.Join(args, " "))
		psArgs := []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait", name, strings.Join(args, " ")),
		}
		if r.Verbose {
			fmt.Printf("Windows Elevation via PS: %s\n", command)
		}
		return exec.Command("powershell", psArgs...).Run()

	default:
		return fmt.Errorf("elevation not supported on %s", runtime.GOOS)
	}
}

// Output executes a command and returns its standard output.
func (r *Runner) Output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
