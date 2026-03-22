package platform

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/lumberjack.v2"
)

// SetupLogger configures the global slog logger.
// For daemon/non-interactive mode, it uses JSON format writing to logFile.
// For interactive mode, it uses text format writing to stderr.
func SetupLogger(level string, logFile string, isInteractive bool) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if isInteractive {
		handler = newColorHandler(os.Stderr, lvl)
	} else {
		var w io.Writer
		if logFile != "" {
			w = &lumberjack.Logger{
				Filename:   logFile,
				MaxSize:    10, // MB
				MaxBackups: 5,
			}
		} else {
			w = os.Stderr
		}
		handler = slog.NewJSONHandler(w, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
