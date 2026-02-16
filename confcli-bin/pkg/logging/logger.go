package logging

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

// Logger provides logging functionality with debug level support
type Logger struct {
	logger *log.Logger
}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	return &Logger{
		logger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

// Debug logs a debug message if debug mode is enabled
func (l *Logger) Debug(format string, args ...interface{}) {
	if viper.GetBool("debug") {
		message := fmt.Sprintf("[DEBUG] "+format, args...)
		l.logger.Output(2, message) // Skip this function in the call stack
	}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	message := fmt.Sprintf("[INFO] "+format, args...)
	l.logger.Output(2, message) // Skip this function in the call stack
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	message := fmt.Sprintf("[WARN] "+format, args...)
	l.logger.Output(2, message) // Skip this function in the call stack
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	message := fmt.Sprintf("[ERROR] "+format, args...)
	l.logger.Output(2, message) // Skip this function in the call stack
}

// IsDebugEnabled returns whether debug logging is enabled
func (l *Logger) IsDebugEnabled() bool {
	return viper.GetBool("debug")
}