package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage encryption keys",
}

var keyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current encryption key pair",
	RunE:  runKeyShow,
}

var keyImportCmd = &cobra.Command{
	Use:   "import <identity>",
	Short: "Import an age identity into keyring or key file",
	Args:  cobra.ExactArgs(1),
	RunE:  runKeyImport,
}

var keyGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a fresh key pair (old encrypted backups become inaccessible)",
	RunE:  runKeyGenerate,
}

func init() {
	keyGenerateCmd.Flags().Bool("force", false, "Skip confirmation prompt")
	keyCmd.AddCommand(keyShowCmd, keyImportCmd, keyGenerateCmd)
	rootCmd.AddCommand(keyCmd)
}

func runKeyShow(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()

	if !appCfg.Encryption.Enabled {
		return fmt.Errorf("encryption is not enabled")
	}

	platform.KeyValue("Public key (recipient)", appCfg.Encryption.Recipient)
	platform.KeyValue("Key source            ", appCfg.Encryption.KeySource)

	identity, err := loadIdentity(appCfg)
	if err != nil {
		platform.Warn(fmt.Sprintf("Private key (identity): NOT FOUND (%v)", err))
		fmt.Println()
		platform.Info("Use 'packrat key import' to re-import your recovery key.")
		return nil
	}

	platform.KeyValue("Private key (identity)", identity)

	// Verify the key pair matches
	derived, err := crypto.RecipientFromIdentity(identity)
	if err == nil && derived != appCfg.Encryption.Recipient {
		fmt.Println()
		platform.Warn("The stored identity does not match the recipient in config!")
		platform.KeyValue("  Config recipient ", appCfg.Encryption.Recipient)
		platform.KeyValue("  Derived recipient", derived)
	}

	return nil
}

func runKeyImport(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()

	identity := strings.TrimSpace(args[0])
	if !strings.HasPrefix(identity, "AGE-SECRET-KEY-") {
		return fmt.Errorf("invalid identity: must start with AGE-SECRET-KEY-")
	}

	recipient, err := crypto.RecipientFromIdentity(identity)
	if err != nil {
		return fmt.Errorf("invalid age identity: %w", err)
	}

	if err := storeIdentity(appCfg, identity); err != nil {
		return err
	}

	appCfg.Encryption.Enabled = true
	appCfg.Encryption.Recipient = recipient
	if err := saveAppConfig(); err != nil {
		return err
	}

	platform.Success("Key imported successfully.")
	platform.KeyValue("  Recipient", recipient)
	platform.KeyValue("  Stored in", appCfg.Encryption.KeySource)
	return nil
}

func runKeyGenerate(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		platform.Warn("Generating a new key pair will make ALL previously encrypted")
		fmt.Println("backups inaccessible unless you still have the old identity key.")
		fmt.Println()
		platform.ConfirmPrompt("Continue? [y/N]")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			platform.Info("Aborted.")
			return nil
		}
	}

	recipient, identity, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generating key pair: %w", err)
	}

	if err := storeIdentity(appCfg, identity); err != nil {
		return err
	}

	appCfg.Encryption.Enabled = true
	appCfg.Encryption.Recipient = recipient
	if err := saveAppConfig(); err != nil {
		return err
	}

	platform.Success("New key pair generated.")
	platform.KeyValue("  Recipient", recipient)
	platform.KeyValue("  Stored in", appCfg.Encryption.KeySource)
	fmt.Println()
	platform.Warn("Save this recovery key somewhere safe:")
	fmt.Printf("  %s\n", identity)
	return nil
}

func loadIdentity(cfg *config.Config) (string, error) {
	switch cfg.Encryption.KeySource {
	case "file":
		return crypto.LoadKeyFromFile(cfg.Encryption.KeyFile)
	case "keyring", "":
		return crypto.RetrieveKey()
	case "prompt":
		return "", fmt.Errorf("prompt mode: identity is not stored")
	default:
		return crypto.RetrieveKey()
	}
}

func storeIdentity(cfg *config.Config, identity string) error {
	switch cfg.Encryption.KeySource {
	case "keyring", "":
		if err := crypto.StoreKey(identity); err != nil {
			return fmt.Errorf("storing key in keyring: %w", err)
		}
	case "file":
		if cfg.Encryption.KeyFile == "" {
			cfg.Encryption.KeyFile = platform.ConfigDir() + "/packrat.key"
		}
		if err := crypto.SaveKeyToFile(cfg.Encryption.KeyFile, identity); err != nil {
			return fmt.Errorf("saving key to file: %w", err)
		}
	default:
		if err := crypto.StoreKey(identity); err != nil {
			return fmt.Errorf("storing key in keyring: %w", err)
		}
	}
	return nil
}

func saveAppConfig() error {
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = platform.DefaultConfigPath()
	}
	if err := config.SaveConfig(cfgPath, appCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}
