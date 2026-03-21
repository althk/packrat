package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/BurntSushi/toml"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print resolved config",
	RunE:  runConfigShow,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config in $EDITOR",
	RunE:  runConfigEdit,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check config for errors",
	RunE:  runConfigValidate,
}

var configAddPathCmd = &cobra.Command{
	Use:   "add-path [path]",
	Short: "Quick-add a path to backup",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigAddPath,
}

func init() {
	configCmd.AddCommand(configShowCmd, configEditCmd, configValidateCmd, configAddPathCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	return toml.NewEncoder(os.Stdout).Encode(appCfg)
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = platform.DefaultConfigPath()
	}

	c := exec.Command(editor, cfgPath)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	if err := config.Validate(appCfg); err != nil {
		return err
	}
	fmt.Println("Configuration is valid. ✓")
	return nil
}

func runConfigAddPath(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	path := args[0]

	// Add to first backup group (dotfiles) or create custom
	found := false
	for i, bg := range appCfg.Backups {
		if bg.Name == "dotfiles" {
			appCfg.Backups[i].Paths = append(appCfg.Backups[i].Paths, path)
			found = true
			break
		}
	}

	if !found {
		// Add to a new "custom" group
		customFound := false
		for i, bg := range appCfg.Backups {
			if bg.Name == "custom" {
				appCfg.Backups[i].Paths = append(appCfg.Backups[i].Paths, path)
				customFound = true
				break
			}
		}
		if !customFound {
			appCfg.Backups = append(appCfg.Backups, config.BackupGroup{
				Name:     "custom",
				Paths:    []string{path},
				Interval: "1h",
			})
		}
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = platform.DefaultConfigPath()
	}
	if err := config.SaveConfig(cfgPath, appCfg); err != nil {
		return err
	}

	fmt.Printf("Added %s to backup config.\n", path)
	return nil
}
