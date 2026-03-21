package shell

import (
	"strings"
	"testing"
	"time"
)

func TestParseBashHistory(t *testing.T) {
	input := "ls -la\ncd /tmp\ngit status\n"
	entries, err := ParseHistory("bash", strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
	if entries[0].Command != "ls -la" {
		t.Errorf("entries[0] = %q, want 'ls -la'", entries[0].Command)
	}
}

func TestParseZshHistory(t *testing.T) {
	input := `: 1710500000:0;ls -la
: 1710500100:0;cd /tmp
: 1710500200:0;git status
`
	entries, err := ParseHistory("zsh", strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d, want 3", len(entries))
	}
	if entries[0].Command != "ls -la" {
		t.Errorf("entries[0].Command = %q, want 'ls -la'", entries[0].Command)
	}
	if entries[0].Timestamp.Unix() != 1710500000 {
		t.Errorf("entries[0].Timestamp = %v", entries[0].Timestamp)
	}
}

func TestParseFishHistory(t *testing.T) {
	input := `- cmd: ls -la
  when: 1710500000
- cmd: cd /tmp
  when: 1710500100
`
	entries, err := ParseHistory("fish", strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].Command != "ls -la" {
		t.Errorf("entries[0].Command = %q", entries[0].Command)
	}
	if entries[0].Timestamp.Unix() != 1710500000 {
		t.Errorf("entries[0].Timestamp = %v", entries[0].Timestamp)
	}
}

func TestMergeHistories(t *testing.T) {
	ts1 := time.Unix(1710500000, 0)
	ts2 := time.Unix(1710500100, 0)
	ts3 := time.Unix(1710500200, 0)

	a := []HistoryEntry{
		{Timestamp: ts1, Command: "ls"},
		{Timestamp: ts2, Command: "cd"},
	}
	b := []HistoryEntry{
		{Timestamp: ts2, Command: "cd"}, // duplicate
		{Timestamp: ts3, Command: "git status"},
	}

	merged := MergeHistories(a, b)
	if len(merged) != 3 {
		t.Fatalf("merged len = %d, want 3", len(merged))
	}

	// Should be sorted by timestamp
	if merged[0].Command != "ls" {
		t.Errorf("merged[0] = %q, want 'ls'", merged[0].Command)
	}
	if merged[2].Command != "git status" {
		t.Errorf("merged[2] = %q, want 'git status'", merged[2].Command)
	}
}

func TestFormatHistory(t *testing.T) {
	ts := time.Unix(1710500000, 0)
	entries := []HistoryEntry{
		{Timestamp: ts, Command: "ls -la"},
	}

	zsh := FormatHistory("zsh", entries)
	if !strings.Contains(zsh, ": 1710500000:0;ls -la") {
		t.Errorf("zsh format = %q", zsh)
	}

	fish := FormatHistory("fish", entries)
	if !strings.Contains(fish, "- cmd: ls -la") {
		t.Errorf("fish format = %q", fish)
	}

	bash := FormatHistory("bash", entries)
	if !strings.Contains(bash, "ls -la") {
		t.Errorf("bash format = %q", bash)
	}
}

func TestDetectShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if s := DetectShell(); s != "zsh" {
		t.Errorf("got %q, want zsh", s)
	}
}
