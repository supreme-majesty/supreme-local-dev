package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	targetUser := "supreme-majesty" // hardcoded for test
	fmt.Println("Testing scraping for user:", targetUser)

	cmd := exec.Command("pgrep", "-u", targetUser)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error running pgrep: %v\n", err)
		return
	}

	pids := strings.Fields(string(output))
	fmt.Printf("Found %d PIDs\n", len(pids))

	for i := len(pids) - 1; i >= 0; i-- {
		pid := pids[i]
		envPath := fmt.Sprintf("/proc/%s/environ", pid)
		content, err := os.ReadFile(envPath)
		if err != nil {
			// fmt.Printf("Skipping PID %s: %v\n", pid, err)
			continue
		}

		envData := string(content)
		if strings.Contains(envData, "DISPLAY=") {
			fmt.Printf("Found DISPLAY in PID %s\n", pid)
			parts := strings.Split(envData, "\x00")
			for _, p := range parts {
				if strings.HasPrefix(p, "DISPLAY=") ||
					strings.HasPrefix(p, "WAYLAND_DISPLAY=") ||
					strings.HasPrefix(p, "XAUTHORITY=") ||
					strings.HasPrefix(p, "DBUS_SESSION_BUS_ADDRESS=") {
					fmt.Println("  ", p)
				}
			}
			return
		}
	}
	fmt.Println("No process with DISPLAY found")
}
