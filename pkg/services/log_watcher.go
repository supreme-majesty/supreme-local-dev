package services

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hpcloud/tail"
	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
	LogLevelUnknown LogLevel = "unknown"
)

// LogSource represents the origin of log entries
type LogSource string

const (
	LogSourceNginxError  LogSource = "nginx-error"
	LogSourceNginxAccess LogSource = "nginx-access"
	LogSourcePHPFPM      LogSource = "php-fpm"
	LogSourceLaravel     LogSource = "laravel"
)

// LogEntryData represents a single log line with metadata
type LogEntryData struct {
	ID        string    `json:"id"`
	Source    LogSource `json:"source"`
	Level     LogLevel  `json:"level"`
	Message   string    `json:"message"`
	Timestamp string    `json:"timestamp"`
	Raw       string    `json:"raw"`
}

// LogWatcher watches multiple log files and broadcasts entries via EventBus
type LogWatcher struct {
	Bus          *events.Bus
	watchers     map[LogSource]*tail.Tail
	mu           sync.RWMutex
	counter      int64
	pathProvider func() map[string]string
}

// NewLogWatcher creates a new log watcher service
func NewLogWatcher(bus *events.Bus, pathProvider func() map[string]string) *LogWatcher {
	return &LogWatcher{
		Bus:          bus,
		watchers:     make(map[LogSource]*tail.Tail),
		pathProvider: pathProvider,
	}
}

// GetAvailableSources returns log sources with their paths
func (w *LogWatcher) GetAvailableSources() map[LogSource]string {
	sources := make(map[LogSource]string)

	if w.pathProvider == nil {
		return sources
	}

	rawPaths := w.pathProvider()

	// Map raw keys to LogSource
	if path, ok := rawPaths["nginx_access"]; ok {
		if _, err := os.Stat(path); err == nil {
			sources[LogSourceNginxAccess] = path
		}
	}
	if path, ok := rawPaths["nginx_error"]; ok {
		if _, err := os.Stat(path); err == nil {
			sources[LogSourceNginxError] = path
		}
	}

	// PHP-FPM
	// Check for various keys or generic
	if path, ok := rawPaths["php_fpm"]; ok {
		if _, err := os.Stat(path); err == nil {
			sources[LogSourcePHPFPM] = path
		}
	} else if path, ok := rawPaths["php_error"]; ok {
		// Start treating php_error as PHP source
		if _, err := os.Stat(path); err == nil {
			sources[LogSourcePHPFPM] = path
		}
	}

	return sources
}

// StartWatching begins tailing a log source
func (w *LogWatcher) StartWatching(source LogSource) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Already watching?
	if _, exists := w.watchers[source]; exists {
		return nil
	}

	sources := w.GetAvailableSources()
	path, ok := sources[source]
	if !ok {
		return fmt.Errorf("log source %s not found", source)
	}

	// Ensure file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("log file does not exist: %s", path)
	}

	t, err := tail.TailFile(path, tail.Config{
		Follow: true,
		ReOpen: true,
		Poll:   true,
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: io.SeekEnd,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tail %s: %w", path, err)
	}

	w.watchers[source] = t

	// Process logs in background
	go w.processLogs(source, t)

	fmt.Printf("Log Watcher: Started watching %s (%s)\n", source, path)
	return nil
}

// StopWatching stops tailing a log source
func (w *LogWatcher) StopWatching(source LogSource) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, exists := w.watchers[source]; exists {
		t.Cleanup()
		t.Stop()
		delete(w.watchers, source)
		fmt.Printf("Log Watcher: Stopped watching %s\n", source)
	}
}

// StopAll stops all log watchers
func (w *LogWatcher) StopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for source, t := range w.watchers {
		t.Cleanup()
		t.Stop()
		fmt.Printf("Log Watcher: Stopped %s\n", source)
	}
	w.watchers = make(map[LogSource]*tail.Tail)
}

// IsWatching checks if a source is being watched
func (w *LogWatcher) IsWatching(source LogSource) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, exists := w.watchers[source]
	return exists
}

// processLogs handles incoming log lines from a tail
func (w *LogWatcher) processLogs(source LogSource, t *tail.Tail) {
	for line := range t.Lines {
		if line.Text == "" {
			continue
		}

		w.mu.Lock()
		w.counter++
		id := fmt.Sprintf("%s-%d-%d", source, time.Now().UnixNano(), w.counter)
		w.mu.Unlock()

		entry := LogEntryData{
			ID:        id,
			Source:    source,
			Level:     w.parseLogLevel(source, line.Text),
			Message:   w.parseMessage(source, line.Text),
			Timestamp: time.Now().Format(time.RFC3339),
			Raw:       line.Text,
		}

		w.Bus.Publish(events.Event{
			Type:    events.LogEntry,
			Payload: entry,
		})
	}
}

// parseLogLevel extracts log level from a log line
func (w *LogWatcher) parseLogLevel(source LogSource, line string) LogLevel {
	lowerLine := strings.ToLower(line)

	switch source {
	case LogSourceNginxError:
		if strings.Contains(lowerLine, "[error]") {
			return LogLevelError
		}
		if strings.Contains(lowerLine, "[warn]") {
			return LogLevelWarning
		}
		if strings.Contains(lowerLine, "[notice]") || strings.Contains(lowerLine, "[info]") {
			return LogLevelInfo
		}
		return LogLevelError // Nginx error log is mostly errors

	case LogSourcePHPFPM:
		if strings.Contains(lowerLine, "fatal") || strings.Contains(lowerLine, "error") {
			return LogLevelError
		}
		if strings.Contains(lowerLine, "warning") || strings.Contains(lowerLine, "warn") {
			return LogLevelWarning
		}
		if strings.Contains(lowerLine, "notice") {
			return LogLevelInfo
		}
		return LogLevelInfo

	case LogSourceLaravel:
		// Laravel logs often start with [YYYY-MM-DD HH:MM:SS] environment.LEVEL:
		if strings.Contains(lowerLine, ".error:") || strings.Contains(lowerLine, ".critical:") ||
			strings.Contains(lowerLine, ".alert:") || strings.Contains(lowerLine, ".emergency:") {
			return LogLevelError
		}
		if strings.Contains(lowerLine, ".warning:") {
			return LogLevelWarning
		}
		if strings.Contains(lowerLine, ".debug:") {
			return LogLevelDebug
		}
		return LogLevelInfo

	case LogSourceNginxAccess:
		// Check HTTP status codes for access logs
		// Look for 4xx or 5xx status codes
		if matched, _ := regexp.MatchString(`"\s[45]\d{2}\s`, line); matched {
			return LogLevelError
		}
		if matched, _ := regexp.MatchString(`"\s3\d{2}\s`, line); matched {
			return LogLevelWarning
		}
		return LogLevelInfo
	}

	return LogLevelUnknown
}

// parseMessage cleans up a log message for display
func (w *LogWatcher) parseMessage(source LogSource, line string) string {
	// For now, return the raw line; could be enhanced per source
	// Truncate very long lines
	if len(line) > 500 {
		return line[:500] + "..."
	}
	return line
}

// GetLastLines returns the last N lines from a log file
func (w *LogWatcher) GetLastLines(source LogSource, n int) ([]LogEntryData, error) {
	sources := w.GetAvailableSources()
	path, ok := sources[source]
	if !ok {
		return nil, fmt.Errorf("log source %s not found", source)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read file and get last N lines
	lines, err := w.tailFile(file, n)
	if err != nil {
		return nil, err
	}

	var entries []LogEntryData
	for i, line := range lines {
		if line == "" {
			continue
		}
		entries = append(entries, LogEntryData{
			ID:        fmt.Sprintf("init-%s-%d", source, i),
			Source:    source,
			Level:     w.parseLogLevel(source, line),
			Message:   w.parseMessage(source, line),
			Timestamp: time.Now().Format(time.RFC3339),
			Raw:       line,
		})
	}

	return entries, nil
}

// tailFile reads the last n lines from a file
func (w *LogWatcher) tailFile(file *os.File, n int) ([]string, error) {
	// Seek to end
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return []string{}, nil
	}

	// Read from end in chunks
	var lines []string
	bufferSize := int64(4096)
	offset := size

	for offset > 0 && len(lines) < n {
		readSize := bufferSize
		if offset < bufferSize {
			readSize = offset
		}
		offset -= readSize

		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, err
		}

		buf := make([]byte, readSize)
		_, err = file.Read(buf)
		if err != nil {
			return nil, err
		}

		// Split into lines
		scanner := bufio.NewScanner(strings.NewReader(string(buf)))
		var chunk []string
		for scanner.Scan() {
			chunk = append(chunk, scanner.Text())
		}

		// Prepend to lines (reverse order)
		lines = append(chunk, lines...)
	}

	// Return only last n lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, nil
}

// WatchLaravelLogs watches Laravel logs for a specific project
func (w *LogWatcher) WatchLaravelLog(projectPath string) error {
	logPath := filepath.Join(projectPath, "storage", "logs", "laravel.log")

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("laravel log not found: %s", logPath)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Use a unique key for Laravel logs per project
	source := LogSource(fmt.Sprintf("laravel:%s", filepath.Base(projectPath)))

	if _, exists := w.watchers[source]; exists {
		return nil
	}

	t, err := tail.TailFile(logPath, tail.Config{
		Follow: true,
		ReOpen: true,
		Poll:   true,
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: io.SeekEnd,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tail %s: %w", logPath, err)
	}

	w.watchers[source] = t
	go w.processLogs(source, t)

	fmt.Printf("Log Watcher: Started watching Laravel log for %s\n", projectPath)
	return nil
}
