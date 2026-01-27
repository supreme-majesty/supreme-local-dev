package services

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

// IssueSeverity indicates how urgent an issue is
type IssueSeverity string

const (
	SeverityInfo     IssueSeverity = "info"
	SeverityWarning  IssueSeverity = "warning"
	SeverityCritical IssueSeverity = "critical"
)

// HealerIssue represents a detected problem
type HealerIssue struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Severity    IssueSeverity `json:"severity"`
	Source      LogSource     `json:"source"` // From log_watcher.go
	DetectedAt  time.Time     `json:"detected_at"`
	FixAction   string        `json:"fix_action"` // Key for the fix function
	CanAutoFix  bool          `json:"can_auto_fix"`
}

// HealerService analyzes logs and offers fixes
type HealerService struct {
	Bus          *events.Bus
	activeIssues map[string]HealerIssue
	mu           sync.RWMutex
	lastAnalyses map[string]time.Time // Debounce mechanism
}

func NewHealerService(bus *events.Bus) *HealerService {
	return &HealerService{
		Bus:          bus,
		activeIssues: make(map[string]HealerIssue),
		lastAnalyses: make(map[string]time.Time),
	}
}

// Start listens to log entries
func (h *HealerService) Start() {
	h.Bus.Subscribe(events.LogEntry, h.handleLogEntry)
	fmt.Println("Supreme Healer: Active and watching for anomalies.")
}

func (h *HealerService) handleLogEntry(e events.Event) {
	entry, ok := e.Payload.(LogEntryData)
	if !ok {
		return
	}

	// Only analyze Error or Warning logs to save CPU
	if entry.Level != LogLevelError && entry.Level != LogLevelWarning {
		return
	}

	h.analyze(entry)
}

func (h *HealerService) analyze(entry LogEntryData) {
	msg := strings.ToLower(entry.Message)

	// 1. Port Conflict (Address already in use)
	if strings.Contains(msg, "address already in use") || strings.Contains(msg, "bind() to") {
		// Detect port if possible (simplified regex or string parsing)
		port := "unknown"
		if strings.Contains(msg, "0.0.0.0:80") || strings.Contains(msg, ":80") {
			port = "80"
		} else if strings.Contains(msg, ":443") {
			port = "443"
		} else if strings.Contains(msg, ":3306") {
			port = "3306"
		}

		h.reportIssue(HealerIssue{
			ID:          fmt.Sprintf("port-conflict-%s", port),
			Title:       fmt.Sprintf("Port %s is Blocked", port),
			Description: fmt.Sprintf("Another service is using port %s, preventing start.", port),
			Severity:    SeverityCritical,
			Source:      entry.Source,
			FixAction:   fmt.Sprintf("kill_port_%s", port),
			CanAutoFix:  true,
		})
		return
	}

	// 2. Missing PHP Extension
	if strings.Contains(msg, "call to undefined function") {
		// Extract function name to guess extension
		// Example: "Call to undefined function imagettftext()" -> gd
		if strings.Contains(msg, "imagettftext") || strings.Contains(msg, "imagecreate") {
			h.reportIssue(HealerIssue{
				ID:          "missing-ext-gd",
				Title:       "Missing PHP Extension: GD",
				Description: "Your code requires the GD image library.",
				Severity:    SeverityWarning,
				Source:      entry.Source,
				FixAction:   "install_ext_gd",
				CanAutoFix:  true, // If we have sudo
			})
		}
		// Add more common ones (curl, mbstring, etc)
		return
	}

	// 3. Permissions Error
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "access denied") {
		// Try to extract path
		// Simplistic extraction logic
		h.reportIssue(HealerIssue{
			ID:          fmt.Sprintf("perm-error-%d", time.Now().Unix()),
			Title:       "Permission Denied",
			Description: "The application cannot write to a file or directory.",
			Severity:    SeverityWarning,
			Source:      entry.Source,
			FixAction:   "fix_permissions_generic",
			CanAutoFix:  false, // Too risky to auto-fix without exact path knowledge
		})
		return
	}
}

func (h *HealerService) reportIssue(issue HealerIssue) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Debounce: Don't report same issue ID more than once every minute
	if last, ok := h.lastAnalyses[issue.ID]; ok {
		if time.Since(last) < 1*time.Minute {
			return
		}
	}
	h.lastAnalyses[issue.ID] = time.Now()

	issue.DetectedAt = time.Now()
	h.activeIssues[issue.ID] = issue

	fmt.Printf("[HEALER] Detected Issue: %s (%s)\n", issue.Title, issue.ID)

	h.Bus.Publish(events.Event{
		Type:    events.HealerIssueDetected,
		Payload: issue,
	})
}

// GetActiveIssues returns all unsolved issues
func (h *HealerService) GetActiveIssues() []HealerIssue {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var list []HealerIssue
	for _, i := range h.activeIssues {
		list = append(list, i)
	}
	return list
}

// ResolveIssue executes the fix
func (h *HealerService) ResolveIssue(issueID string) error {
	h.mu.RLock()
	issue, ok := h.activeIssues[issueID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("issue not found or already resolved")
	}

	fmt.Printf("[HEALER] Attempting to fix: %s\n", issue.Title)

	var err error
	switch {
	case strings.HasPrefix(issue.FixAction, "kill_port_"):
		port := strings.TrimPrefix(issue.FixAction, "kill_port_")
		err = h.killProcessOnPort(port)
	case issue.FixAction == "install_ext_gd":
		err = h.installPackage("php-gd")
	case issue.FixAction == "fix_permissions_generic":
		// No-op or guide user
		return fmt.Errorf("automatic permission fix not yet implemented for safety")
	default:
		return fmt.Errorf("unknown fix action: %s", issue.FixAction)
	}

	if err != nil {
		return err
	}

	// Mark resolved
	h.mu.Lock()
	delete(h.activeIssues, issueID)
	h.mu.Unlock()

	h.Bus.Publish(events.Event{
		Type:    events.HealerIssueResolved,
		Payload: issueID,
	})

	return nil
}

func (h *HealerService) killProcessOnPort(port string) error {
	// fuser -k 80/tcp
	cmd := exec.Command("fuser", "-k", fmt.Sprintf("%s/tcp", port))
	// Might need sudo if we are not root (daemon usually is root or has caps)
	return cmd.Run()
}

func (h *HealerService) installPackage(pkg string) error {
	// Assumes apt-get for now (User's OS is Linux)
	// DEBIAN_FRONTEND=noninteractive
	cmd := exec.Command("apt-get", "install", "-y", pkg)
	return cmd.Run()
}
