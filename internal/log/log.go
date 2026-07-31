package log

import (
	"os"

	"github.com/charmbracelet/log"
)

var logger = log.NewWithOptions(os.Stderr, log.Options{
	ReportCaller:    true,
	ReportTimestamp: true,
	Prefix:          "DS-Proxy 🚀",
	Level:           log.InfoLevel,
})

// Init configures the default logger level.
func Init(verbose bool) {
	if verbose {
		logger.SetLevel(log.DebugLevel)
	} else {
		logger.SetLevel(log.InfoLevel)
	}
}

// Info logs at Info level.
func Info(msg string, keyvals ...any) {
	logger.Info(msg, keyvals...)
}

// Warn logs at Warn level.
func Warn(msg string, keyvals ...any) {
	logger.Warn(msg, keyvals...)
}

// Error logs at Error level.
func Error(msg string, keyvals ...any) {
	logger.Error(msg, keyvals...)
}

// Debug logs at Debug level.
func Debug(msg string, keyvals ...any) {
	logger.Debug(msg, keyvals...)
}
