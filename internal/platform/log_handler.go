package platform

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// colorHandler is a slog.Handler that outputs colored, human-friendly log lines.
type colorHandler struct {
	w     io.Writer
	level slog.Level
	mu    sync.Mutex
}

var (
	logDebug = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	logInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	logWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	logError = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

func newColorHandler(w io.Writer, level slog.Level) *colorHandler {
	return &colorHandler{w: w, level: level}
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	var prefix string
	var style lipgloss.Style

	switch {
	case r.Level >= slog.LevelError:
		prefix = "ERROR"
		style = logError
	case r.Level >= slog.LevelWarn:
		prefix = " WARN"
		style = logWarn
	case r.Level >= slog.LevelInfo:
		prefix = " INFO"
		style = logInfo
	default:
		prefix = "DEBUG"
		style = logDebug
	}

	tag := style.Render(prefix)
	msg := r.Message

	// Collect attributes
	var attrs string
	r.Attrs(func(a slog.Attr) bool {
		key := logDebug.Render(a.Key + "=")
		attrs += " " + key + fmt.Sprintf("%v", a.Value.Any())
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.w, "%s %s%s\n", tag, msg, attrs)
	return nil
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return h
}
