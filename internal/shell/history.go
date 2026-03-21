package shell

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HistoryEntry represents a single shell history entry.
type HistoryEntry struct {
	Timestamp time.Time
	Command   string
}

// DetectShell returns the current shell name.
func DetectShell() string {
	shell := os.Getenv("SHELL")
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		return "zsh"
	case "fish":
		return "fish"
	default:
		return "bash"
	}
}

// HistoryPath returns the history file path for the given shell.
func HistoryPath(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zsh_history")
	case "fish":
		return filepath.Join(home, ".local", "share", "fish", "fish_history")
	default:
		return filepath.Join(home, ".bash_history")
	}
}

// ParseHistory parses shell history from the given reader.
func ParseHistory(shell string, reader io.Reader) ([]HistoryEntry, error) {
	switch shell {
	case "zsh":
		return parseZshHistory(reader)
	case "fish":
		return parseFishHistory(reader)
	default:
		return parseBashHistory(reader)
	}
}

// parseBashHistory parses bash history (one command per line, no timestamps).
func parseBashHistory(reader io.Reader) ([]HistoryEntry, error) {
	var entries []HistoryEntry
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		entries = append(entries, HistoryEntry{Command: line})
	}
	return entries, scanner.Err()
}

// parseZshHistory parses zsh extended history format.
// Format: ": timestamp:0;command"
func parseZshHistory(reader io.Reader) ([]HistoryEntry, error) {
	var entries []HistoryEntry
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Try extended format: ": timestamp:duration;command"
		if strings.HasPrefix(line, ": ") {
			rest := line[2:]
			colonIdx := strings.Index(rest, ":")
			if colonIdx > 0 {
				tsStr := rest[:colonIdx]
				ts, err := strconv.ParseInt(tsStr, 10, 64)
				if err == nil {
					semiIdx := strings.Index(rest[colonIdx:], ";")
					if semiIdx >= 0 {
						cmd := rest[colonIdx+semiIdx+1:]
						entries = append(entries, HistoryEntry{
							Timestamp: time.Unix(ts, 0),
							Command:   cmd,
						})
						continue
					}
				}
			}
		}

		// Fallback: plain command
		entries = append(entries, HistoryEntry{Command: line})
	}
	return entries, scanner.Err()
}

// parseFishHistory parses fish shell history (YAML-like format).
// Format:
// - cmd: command
//   when: timestamp
func parseFishHistory(reader io.Reader) ([]HistoryEntry, error) {
	var entries []HistoryEntry
	scanner := bufio.NewScanner(reader)

	var current HistoryEntry
	var hasCmd bool

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "- cmd: ") {
			if hasCmd {
				entries = append(entries, current)
			}
			current = HistoryEntry{Command: strings.TrimPrefix(line, "- cmd: ")}
			hasCmd = true
		} else if strings.HasPrefix(line, "  when: ") && hasCmd {
			tsStr := strings.TrimPrefix(line, "  when: ")
			ts, err := strconv.ParseInt(tsStr, 10, 64)
			if err == nil {
				current.Timestamp = time.Unix(ts, 0)
			}
		}
	}
	if hasCmd {
		entries = append(entries, current)
	}

	return entries, scanner.Err()
}

// MergeHistories merges two history lists, deduplicating by timestamp+command.
func MergeHistories(a, b []HistoryEntry) []HistoryEntry {
	type key struct {
		ts  int64
		cmd string
	}
	seen := make(map[key]bool)
	var merged []HistoryEntry

	add := func(entries []HistoryEntry) {
		for _, e := range entries {
			k := key{ts: e.Timestamp.Unix(), cmd: e.Command}
			if !seen[k] {
				seen[k] = true
				merged = append(merged, e)
			}
		}
	}

	add(a)
	add(b)

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Timestamp.Equal(merged[j].Timestamp) {
			return merged[i].Command < merged[j].Command
		}
		return merged[i].Timestamp.Before(merged[j].Timestamp)
	})

	return merged
}

// FormatHistory formats history entries back to the given shell format.
func FormatHistory(shell string, entries []HistoryEntry) string {
	var b strings.Builder
	for _, e := range entries {
		switch shell {
		case "zsh":
			if !e.Timestamp.IsZero() {
				fmt.Fprintf(&b, ": %d:0;%s\n", e.Timestamp.Unix(), e.Command)
			} else {
				fmt.Fprintf(&b, "%s\n", e.Command)
			}
		case "fish":
			fmt.Fprintf(&b, "- cmd: %s\n", e.Command)
			if !e.Timestamp.IsZero() {
				fmt.Fprintf(&b, "  when: %d\n", e.Timestamp.Unix())
			}
		default:
			fmt.Fprintf(&b, "%s\n", e.Command)
		}
	}
	return b.String()
}
