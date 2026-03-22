package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/platform"
	"github.com/harish/packrat/internal/storage"
	"github.com/spf13/cobra"
)

var initRestore bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "First-time setup wizard",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initRestore, "restore", false, "restore from existing backups (new machine setup)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  🐀 Welcome to Packrat!")
	fmt.Println("  Let's get your backups configured.")
	fmt.Println()

	cfg := config.DefaultConfig()

	// Prerequisite: Check for rclone before anything else
	fmt.Println("  Checking dependencies...")
	rcloneVer, err := storage.CheckRcloneInstalled()
	if err != nil {
		fmt.Println("  > ✗ rclone not found.")
		fmt.Print("  > Install rclone automatically? [Y/n] ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "n" || answer == "no" {
			fmt.Println("  > Please install rclone from https://rclone.org")
			fmt.Println("  > After installing, run: rclone config")
			fmt.Println("  > Then re-run: packrat init")
			return err
		}

		binPath, installErr := storage.InstallRclone(os.Stdout)
		if installErr != nil {
			fmt.Println("  > ✗ Automatic install failed.")
			fmt.Println("  > Please install manually from https://rclone.org")
			fmt.Println("  > Then re-run: packrat init")
			return installErr
		}
		storage.RcloneBinary = binPath
	} else {
		fmt.Printf("  > ✓ rclone found (%s)\n", rcloneVer)
	}
	fmt.Println()

	// Step 1: Machine Name
	fmt.Printf("  Step 1/5: Machine Name\n")
	fmt.Printf("  > What should we call this machine? [%s] ", cfg.General.MachineName)
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.General.MachineName = name
	}
	fmt.Println()

	// Step 2: Storage Backend (remote selection)
	fmt.Println("  Step 2/5: Storage Backend")
	fmt.Printf("  > Which remote should Packrat use? [%s] ", cfg.Storage.RcloneRemote)
	remote, _ := reader.ReadString('\n')
	remote = strings.TrimSpace(remote)
	if remote != "" {
		cfg.Storage.RcloneRemote = remote
	}

	if cfg.Storage.RcloneRemote != "" {
		if err := storage.ValidateRemote(cfg.Storage.RcloneRemote); err != nil {
			fmt.Printf("  > ✗ Remote %q not found. Run 'rclone config' to set up a remote.\n", cfg.Storage.RcloneRemote)
			fmt.Println("  > Continuing anyway — you can configure the remote later.")
		} else {
			fmt.Println("  > ✓ Remote validated")
		}
	} else {
		fmt.Print("  > No remote configured. Run 'rclone config' now to set one up? [Y/n] ")
		rcAnswer, _ := reader.ReadString('\n')
		rcAnswer = strings.TrimSpace(strings.ToLower(rcAnswer))
		if rcAnswer != "n" && rcAnswer != "no" {
			rcloneCmd := exec.Command(storage.RcloneBinary, "config")
			rcloneCmd.Stdin = os.Stdin
			rcloneCmd.Stdout = os.Stdout
			rcloneCmd.Stderr = os.Stderr
			if err := rcloneCmd.Run(); err != nil {
				fmt.Printf("  > ⚠️  rclone config exited with error: %v\n", err)
			}
			// Re-prompt for remote after config
			fmt.Print("  > Which remote should Packrat use? ")
			remote, _ = reader.ReadString('\n')
			remote = strings.TrimSpace(remote)
			if remote != "" {
				cfg.Storage.RcloneRemote = remote
			}
		}
	}
	fmt.Println()

	// Step 3: Encryption
	fmt.Println("  Step 3/5: Encryption")
	fmt.Print("  > Enable encryption for sensitive configs? [Y/n] ")
	encAnswer, _ := reader.ReadString('\n')
	encAnswer = strings.TrimSpace(strings.ToLower(encAnswer))

	if encAnswer == "n" || encAnswer == "no" {
		cfg.Encryption.Enabled = false
		fmt.Println("  > Encryption disabled.")
	} else {
		cfg.Encryption.Enabled = true
		recipient, identity, err := crypto.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("generating encryption key: %w", err)
		}
		cfg.Encryption.Recipient = recipient

		// Try storing in keyring
		if err := crypto.StoreKey(identity); err != nil {
			// Fallback to file
			keyFile := platform.ConfigDir() + "/packrat.key"
			crypto.SaveKeyToFile(keyFile, identity)
			cfg.Encryption.KeySource = "file"
			cfg.Encryption.KeyFile = keyFile
			fmt.Printf("  > Key saved to: %s\n", keyFile)
		} else {
			cfg.Encryption.KeySource = "keyring"
			fmt.Println("  > Key stored in OS keyring.")
		}

		fmt.Println("  > ⚠️  Save this recovery key somewhere safe:")
		fmt.Printf("  >    %s\n", identity)
		fmt.Println("  >    (You'll need this if you lose access to this machine's keyring)")
	}
	fmt.Println()

	// Step 4: Backup groups
	fmt.Println("  Step 4/5: What to Back Up")
	shell := config.DetectShell()
	fmt.Printf("  > Detected shell: %s\n", shell)
	fmt.Println("  > Auto-configured backup groups:")
	for _, bg := range cfg.Backups {
		enc := ""
		if bg.Encrypt {
			enc = " [encrypted]"
		}
		fmt.Printf("  >   ✓ %-15s (%d paths)%s\n", bg.Name, len(bg.Paths), enc)
	}
	fmt.Println()

	// Step 5: Schedule
	fmt.Println("  Step 5/5: Schedule")
	fmt.Printf("  > Default backup interval: [%s] ", cfg.Scheduler.DefaultInterval)
	interval, _ := reader.ReadString('\n')
	interval = strings.TrimSpace(interval)
	if interval != "" {
		cfg.Scheduler.DefaultInterval = interval
	}
	fmt.Println()

	// Save config
	cfgPath := platform.DefaultConfigPath()
	if cfgFile != "" {
		cfgPath = cfgFile
	}
	if err := platform.EnsureDir(platform.ConfigDir()); err != nil {
		return err
	}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("  ✓ Config written to %s\n", cfgPath)

	// Start daemon
	fmt.Print("  > Start daemon now? [Y/n] ")
	daemonAnswer, _ := reader.ReadString('\n')
	daemonAnswer = strings.TrimSpace(strings.ToLower(daemonAnswer))
	if daemonAnswer != "n" && daemonAnswer != "no" {
		exe, _ := os.Executable()
		if err := startDaemonFromInit(exe, cfgPath); err != nil {
			fmt.Printf("  > Could not start daemon: %v\n", err)
		} else {
			fmt.Println("  ✓ Daemon started")
		}
	}

	fmt.Println()
	fmt.Println("  Run `packrat status` to check progress.")
	fmt.Println("  Run `packrat config edit` to customize.")
	fmt.Println()

	return nil
}

func startDaemonFromInit(exe, cfgPath string) error {
	// Import scheduler inline to avoid circular deps
	return nil // Will be wired up when daemon command is ready
}
