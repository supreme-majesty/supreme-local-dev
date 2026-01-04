package services

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

// ArtisanService handles Laravel Artisan command execution with streaming output
type ArtisanService struct {
	events *events.Bus
}

// ArtisanOutput represents a line of command output
type ArtisanOutput struct {
	ProjectPath string `json:"project_path"`
	Line        string `json:"line"`
	IsError     bool   `json:"is_error"`
	Timestamp   int64  `json:"timestamp"`
}

// ArtisanDone signals command completion
type ArtisanDone struct {
	ProjectPath string `json:"project_path"`
	Success     bool   `json:"success"`
	ExitCode    int    `json:"exit_code"`
}

// NewArtisanService creates a new Artisan service
func NewArtisanService(eventBus *events.Bus) *ArtisanService {
	return &ArtisanService{
		events: eventBus,
	}
}

// RunCommand executes an artisan command and streams output via events
func (s *ArtisanService) RunCommand(projectPath, command string) error {
	// Verify artisan exists
	artisanPath := filepath.Join(projectPath, "artisan")

	// Build the command
	args := []string{artisanPath}
	args = append(args, parseCommandArgs(command)...)

	cmd := exec.Command("php", args...)
	cmd.Dir = projectPath

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Get stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			s.events.Publish(events.Event{
				Type: events.ArtisanOutput,
				Payload: ArtisanOutput{
					ProjectPath: projectPath,
					Line:        scanner.Text(),
					IsError:     false,
					Timestamp:   time.Now().UnixMilli(),
				},
			})
		}
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			s.events.Publish(events.Event{
				Type: events.ArtisanOutput,
				Payload: ArtisanOutput{
					ProjectPath: projectPath,
					Line:        scanner.Text(),
					IsError:     true,
					Timestamp:   time.Now().UnixMilli(),
				},
			})
		}
	}()

	// Wait for command to complete
	err = cmd.Wait()
	exitCode := 0
	success := true
	if err != nil {
		success = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Signal completion
	s.events.Publish(events.Event{
		Type: events.ArtisanDone,
		Payload: ArtisanDone{
			ProjectPath: projectPath,
			Success:     success,
			ExitCode:    exitCode,
		},
	})

	return nil
}

// parseCommandArgs splits a command string into arguments
func parseCommandArgs(command string) []string {
	// Simple tokenizer - handles basic quoting
	var args []string
	var current string
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range command {
		switch {
		case ch == '"' || ch == '\'':
			if inQuote && ch == quoteChar {
				inQuote = false
				quoteChar = 0
			} else if !inQuote {
				inQuote = true
				quoteChar = ch
			} else {
				current += string(ch)
			}
		case ch == ' ' && !inQuote:
			if current != "" {
				args = append(args, current)
				current = ""
			}
		default:
			current += string(ch)
		}
	}
	if current != "" {
		args = append(args, current)
	}

	return args
}

// GetCommonCommands returns a list of commonly used Artisan commands
func (s *ArtisanService) GetCommonCommands() []string {
	return []string{
		"migrate",
		"migrate:fresh",
		"migrate:rollback",
		"db:seed",
		"cache:clear",
		"config:cache",
		"config:clear",
		"route:list",
		"route:cache",
		"route:clear",
		"view:clear",
		"optimize",
		"optimize:clear",
		"queue:work",
		"schedule:run",
		"tinker",
	}
}
