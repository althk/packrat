package crypto

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "packrat"
	keyringAccount = "age-identity"
)

// StoreKey stores the age identity in the OS keyring.
func StoreKey(identity string) error {
	if err := keyring.Set(keyringService, keyringAccount, identity); err != nil {
		return fmt.Errorf("storing key in keyring: %w", err)
	}
	return nil
}

// RetrieveKey retrieves the age identity from the OS keyring.
func RetrieveKey() (string, error) {
	secret, err := keyring.Get(keyringService, keyringAccount)
	if err != nil {
		return "", fmt.Errorf("retrieving key from keyring: %w", err)
	}
	return secret, nil
}

// DeleteKey removes the age identity from the OS keyring.
func DeleteKey() error {
	if err := keyring.Delete(keyringService, keyringAccount); err != nil {
		return fmt.Errorf("deleting key from keyring: %w", err)
	}
	return nil
}

// LoadKeyFromFile reads an age identity from a file.
func LoadKeyFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading key file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveKeyToFile writes an age identity to a file with restrictive permissions.
func SaveKeyToFile(path, identity string) error {
	if err := os.WriteFile(path, []byte(identity+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}
	return nil
}
