// Package platform provides cross-platform utilities for paths, logging, errors, and terminal UI.
package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Terminal color styles using lipgloss.
var (
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	styleAdded    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleModified = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleDeleted  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleTagOK         = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleTagOverdue    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleTagPending    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleTagFailure    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleTagInProgress = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)

	styleKey   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleValue = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

// colorEnabled tracks whether colored output is enabled.
var colorEnabled = true

// SetColorEnabled controls whether UI output uses ANSI colors.
func SetColorEnabled(enabled bool) {
	colorEnabled = enabled
	if !enabled {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// render applies a style only if color is enabled.
func render(style lipgloss.Style, s string) string {
	if !colorEnabled {
		return s
	}
	return style.Render(s)
}

// --- Prefixed message printers ---

// Success prints a green success message to stdout.
func Success(msg string) {
	fmt.Fprintf(os.Stdout, "%s %s\n", render(styleSuccess, "✓"), msg)
}

// Error prints a red error message to stderr.
func Error(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", render(styleError, "✗"), msg)
}

// Warn prints a yellow warning message to stderr.
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", render(styleWarning, "!"), msg)
}

// Info prints a blue info message to stdout.
func Info(msg string) {
	fmt.Fprintf(os.Stdout, "%s %s\n", render(styleInfo, "→"), msg)
}

// Infof prints a formatted blue info message to stdout.
func Infof(format string, args ...interface{}) {
	Info(fmt.Sprintf(format, args...))
}

// Header prints a bold styled header.
func Header(msg string) {
	fmt.Fprintln(os.Stdout, render(styleHeader, msg))
}

// Dim prints dimmed text.
func Dim(msg string) {
	fmt.Fprintln(os.Stdout, render(styleDim, msg))
}

// Bold prints bold text.
func Bold(msg string) string {
	return render(styleBold, msg)
}

// --- Status tags ---

// StatusTag returns a colored status string.
func StatusTag(status string) string {
	switch status {
	case "ok", "success":
		return render(styleTagOK, status)
	case "overdue":
		return render(styleTagOverdue, status)
	case "pending":
		return render(styleTagPending, status)
	case "failure":
		return render(styleTagFailure, status)
	case "in-progress":
		return render(styleTagInProgress, status)
	default:
		return status
	}
}

// --- Diff / file change markers ---

// FileChange prints a colored diff-style file change line.
func FileChange(status, path string) {
	switch status {
	case "added":
		fmt.Printf("  %s %s\n", render(styleAdded, "+"), render(styleAdded, path))
	case "deleted":
		fmt.Printf("  %s %s\n", render(styleDeleted, "-"), render(styleDeleted, path))
	case "modified":
		fmt.Printf("  %s %s\n", render(styleModified, "~"), render(styleModified, path))
	default:
		fmt.Printf("    %s\n", path)
	}
}

// --- Table helpers ---

// TableHeader prints a styled table header row.
func TableHeader(cols ...TableCol) {
	var headerParts, separatorParts []string
	for _, c := range cols {
		format := fmt.Sprintf("%%-%ds", c.Width)
		headerParts = append(headerParts, fmt.Sprintf(format, c.Name))
		separatorParts = append(separatorParts, fmt.Sprintf(format, strings.Repeat("─", len(c.Name))))
	}
	fmt.Println(render(styleBold, strings.Join(headerParts, " ")))
	fmt.Println(render(styleDim, strings.Join(separatorParts, " ")))
}

// TableCol describes a table column.
type TableCol struct {
	Name  string
	Width int
}

// TableRow prints a table row with values.
func TableRow(cols []TableCol, values ...string) {
	var parts []string
	for i, c := range cols {
		format := fmt.Sprintf("%%-%ds", c.Width)
		val := ""
		if i < len(values) {
			val = values[i]
		}
		parts = append(parts, fmt.Sprintf(format, val))
	}
	fmt.Println(strings.Join(parts, " "))
}

// --- Key-value display ---

// KeyValue prints a dim key with a normal value.
func KeyValue(key, value string) {
	fmt.Printf("%s %s\n", render(styleKey, key+":"), render(styleValue, value))
}

// --- Confirmation prompt ---

// ConfirmPrompt prints a styled confirmation prompt.
func ConfirmPrompt(msg string) {
	fmt.Print(render(styleWarning, "? ") + msg + " ")
}

// Itoa converts an int to a string (convenience for table rows).
func Itoa(n int) string {
	return strconv.Itoa(n)
}
