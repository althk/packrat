package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var logLines int

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the log file",
	RunE:  runLog,
}

func init() {
	logCmd.Flags().IntVar(&logLines, "lines", 20, "number of lines to show")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	logPath := platform.LogFilePath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found yet.")
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	start := len(lines) - logLines
	if start < 0 {
		start = 0
	}

	for _, line := range lines[start:] {
		if line != "" {
			fmt.Println(line)
		}
	}
	return nil
}
