package main

import (
	"fmt"

	"github.com/harish/packrat/internal/backup"
	"github.com/spf13/cobra"
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Run garbage collection on old snapshots",
	RunE:  runGC,
}

func init() {
	rootCmd.AddCommand(gcCmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	store := newStorageBackend()
	engine := backup.NewEngine(appCfg, store, stateDB)

	if err := engine.GarbageCollect(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("Garbage collection complete.")
	return nil
}
