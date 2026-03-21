package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var (
	nukeRemote bool
	nukeLocal  bool
)

var nukeCmd = &cobra.Command{
	Use:   "nuke",
	Short: "Delete all packrat data",
	RunE:  runNuke,
}

func init() {
	nukeCmd.Flags().BoolVar(&nukeRemote, "remote", false, "delete all remote data")
	nukeCmd.Flags().BoolVar(&nukeLocal, "local", false, "delete all local packrat data")
	rootCmd.AddCommand(nukeCmd)
}

func runNuke(cmd *cobra.Command, args []string) error {
	if !nukeRemote && !nukeLocal {
		return fmt.Errorf("specify --remote and/or --local")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Are you sure? This cannot be undone. Type 'yes' to confirm: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	if nukeLocal {
		dataDir := platform.DataDir()
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("removing local data: %w", err)
		}
		fmt.Printf("Removed local data: %s\n", dataDir)
	}

	if nukeRemote {
		if err := loadConfig(); err != nil {
			return err
		}
		// Use rclone to delete remote data
		store := newStorageBackend()
		ctx := cmd.Context()

		prefix := appCfg.General.MachineID + "/"
		entries, err := store.List(ctx, prefix)
		if err != nil {
			return fmt.Errorf("listing remote data: %w", err)
		}

		for _, e := range entries {
			if !e.IsDir {
				store.Delete(ctx, e.Path)
			}
		}
		fmt.Printf("Removed remote data for machine %s\n", appCfg.General.MachineID)
	}

	return nil
}
