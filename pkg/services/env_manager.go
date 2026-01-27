package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EnvManager handles .env file operations for projects
type EnvManager struct{}

// EnvFile represents a parsed .env file
type EnvFile struct {
	Path      string            `json:"path"`
	Name      string            `json:"name"`
	Variables map[string]string `json:"variables"`
	ModTime   time.Time         `json:"mod_time"`
}

// EnvBackup represents a backup of an .env file
type EnvBackup struct {
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

// NewEnvManager creates a new environment manager
func NewEnvManager() *EnvManager {
	return &EnvManager{}
}

// ListEnvFiles finds all .env files in a project directory
func (em *EnvManager) ListEnvFiles(projectPath string) ([]EnvFile, error) {
	var envFiles []EnvFile

	// Common .env file patterns
	patterns := []string{".env", ".env.local", ".env.example", ".env.testing", ".env.production"}

	for _, pattern := range patterns {
		path := filepath.Join(projectPath, pattern)
		if info, err := os.Stat(path); err == nil {
			envFiles = append(envFiles, EnvFile{
				Path:    path,
				Name:    pattern,
				ModTime: info.ModTime(),
			})
		}
	}

	return envFiles, nil
}

// ReadEnvFile parses an .env file and returns key-value pairs
func (em *EnvManager) ReadEnvFile(filePath string) (*EnvFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	info, _ := file.Stat()

	variables := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Remove surrounding quotes if present
			if len(value) >= 2 {
				if (value[0] == '"' && value[len(value)-1] == '"') ||
					(value[0] == '\'' && value[len(value)-1] == '\'') {
					value = value[1 : len(value)-1]
				}
			}

			variables[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}

	return &EnvFile{
		Path:      filePath,
		Name:      filepath.Base(filePath),
		Variables: variables,
		ModTime:   info.ModTime(),
	}, nil
}

// WriteEnvFile writes variables to an .env file, creating a backup first
func (em *EnvManager) WriteEnvFile(filePath string, variables map[string]string) error {
	// Create backup before writing
	if _, err := os.Stat(filePath); err == nil {
		if err := em.CreateBackup(filePath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(variables))
	for k := range variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build file content
	var builder strings.Builder
	for _, key := range keys {
		value := variables[key]
		// Quote values containing spaces or special characters
		if strings.ContainsAny(value, " \t\n\"'$") {
			value = fmt.Sprintf("\"%s\"", strings.ReplaceAll(value, "\"", "\\\""))
		}
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	// Write to file
	if err := os.WriteFile(filePath, []byte(builder.String()), 0644); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	return nil
}

// CreateBackup creates a timestamped backup of an .env file
func (em *EnvManager) CreateBackup(filePath string) error {
	// Read original content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Create backup directory
	backupDir := filepath.Join(filepath.Dir(filePath), ".env-backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Generate backup filename
	baseName := filepath.Base(filePath)
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s.%s.bak", baseName, timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	// Write backup
	return os.WriteFile(backupPath, content, 0644)
}

// ListBackups lists all backups for an .env file
func (em *EnvManager) ListBackups(filePath string) ([]EnvBackup, error) {
	backupDir := filepath.Join(filepath.Dir(filePath), ".env-backups")
	baseName := filepath.Base(filePath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []EnvBackup{}, nil
		}
		return nil, err
	}

	var backups []EnvBackup
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), baseName+".") && strings.HasSuffix(entry.Name(), ".bak") {
			info, _ := entry.Info()
			backups = append(backups, EnvBackup{
				Filename:  entry.Name(),
				Path:      filepath.Join(backupDir, entry.Name()),
				CreatedAt: info.ModTime(),
				Size:      info.Size(),
			})
		}
	}

	// Sort by date descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// RestoreBackup restores an .env file from a backup
func (em *EnvManager) RestoreBackup(backupPath, targetPath string) error {
	// Create a backup of current file first
	if _, err := os.Stat(targetPath); err == nil {
		if err := em.CreateBackup(targetPath); err != nil {
			return fmt.Errorf("failed to backup current file: %w", err)
		}
	}

	// Read backup content
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Write to target
	return os.WriteFile(targetPath, content, 0644)
}
