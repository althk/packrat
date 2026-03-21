package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/harish/packrat/internal/backup"
	"github.com/harish/packrat/internal/config"
	"github.com/harish/packrat/internal/crypto"
	"github.com/harish/packrat/internal/platform"
	"github.com/spf13/cobra"
)

var rotateKeyCmd = &cobra.Command{
	Use:   "rotate-key",
	Short: "Generate a new encryption key and re-encrypt all blobs",
	RunE:  runRotateKey,
}

func init() {
	rootCmd.AddCommand(rotateKeyCmd)
}

func runRotateKey(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}
	setupLogger()
	if err := openStateDB(); err != nil {
		return err
	}
	defer stateDB.Close()

	if !appCfg.Encryption.Enabled {
		return fmt.Errorf("encryption is not enabled")
	}

	// Get old identity for decryption
	oldIdentity, err := getIdentityForRotation()
	if err != nil {
		return fmt.Errorf("getting current key: %w", err)
	}

	// Generate new key pair
	newRecipient, newIdentity, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generating new key: %w", err)
	}

	fmt.Println("New key generated. Re-encrypting blobs...")

	store := newStorageBackend()
	ctx := context.Background()

	// Find all encrypted blobs by scanning snapshots
	snapshots, err := stateDB.ListSnapshots("")
	if err != nil {
		return err
	}

	reEncrypted := 0
	for _, snap := range snapshots {
		for _, entry := range snap.Files {
			if !entry.Encrypted {
				continue
			}

			// Download encrypted blob
			oldBlobPath := snap.MachineID + "/" + backup.BlobPath(entry.SHA256) + ".age"
			var buf bytes.Buffer
			if err := store.Download(ctx, oldBlobPath, &buf); err != nil {
				fmt.Printf("  Warning: could not download %s: %v\n", oldBlobPath, err)
				continue
			}

			// Decrypt with old key
			decrypted, err := crypto.Decrypt(&buf, oldIdentity)
			if err != nil {
				fmt.Printf("  Warning: could not decrypt %s: %v\n", entry.Path, err)
				continue
			}

			// Re-encrypt with new key
			decData, _ := readAllBytes(decrypted)
			reEncrypted, err := crypto.Encrypt(bytes.NewReader(decData), newRecipient)
			if err != nil {
				return fmt.Errorf("re-encrypting %s: %w", entry.Path, err)
			}

			// Upload with new encryption
			reEncData, _ := readAllBytes(reEncrypted)
			if err := store.Upload(ctx, oldBlobPath, bytes.NewReader(reEncData)); err != nil {
				return fmt.Errorf("uploading re-encrypted %s: %w", entry.Path, err)
			}
			_ = reEncrypted
		}
	}

	// Store new key
	switch appCfg.Encryption.KeySource {
	case "keyring":
		if err := crypto.StoreKey(newIdentity); err != nil {
			return fmt.Errorf("storing new key: %w", err)
		}
	case "file":
		if err := crypto.SaveKeyToFile(appCfg.Encryption.KeyFile, newIdentity); err != nil {
			return fmt.Errorf("saving new key: %w", err)
		}
	}

	// Update config with new recipient
	appCfg.Encryption.Recipient = newRecipient
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = platform.DefaultConfigPath()
	}
	if err := config.SaveConfig(cfgPath, appCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Key rotation complete. %d blobs re-encrypted.\n", reEncrypted)
	fmt.Println("Save this recovery key somewhere safe:")
	fmt.Printf("  %s\n", newIdentity)

	return nil
}

func getIdentityForRotation() (string, error) {
	switch appCfg.Encryption.KeySource {
	case "file":
		return crypto.LoadKeyFromFile(appCfg.Encryption.KeyFile)
	case "keyring":
		return crypto.RetrieveKey()
	default:
		return crypto.RetrieveKey()
	}
}

func readAllBytes(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
