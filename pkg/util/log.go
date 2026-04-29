package util

import (
	"fmt"
	"time"
)

type LogLevel int

const (
	LevelInfo LogLevel = iota
	LevelWarn
	LevelError
	LevelSuccess
)

// Logger provides stylized output for the CLI and installers.
type Logger struct{}

func (l *Logger) Info(format string, a ...interface{}) {
	l.log(LevelInfo, format, a...)
}

func (l *Logger) Warn(format string, a ...interface{}) {
	l.log(LevelWarn, format, a...)
}

func (l *Logger) Error(format string, a ...interface{}) {
	l.log(LevelError, format, a...)
}

func (l *Logger) Success(format string, a ...interface{}) {
	l.log(LevelSuccess, format, a...)
}

func (l *Logger) log(level LogLevel, format string, a ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	var prefix string
	
	switch level {
	case LevelInfo:
		prefix = "  [INFO] "
	case LevelWarn:
		prefix = "⚠️  [WARN] "
	case LevelError:
		prefix = "❌ [ERR ] "
	case LevelSuccess:
		prefix = "✅ [OK  ] "
	}

	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s %s%s\n", timestamp, prefix, msg)
}
