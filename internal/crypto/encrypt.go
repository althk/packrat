package crypto

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

// GenerateKeyPair generates a new age X25519 key pair.
// Returns the public key (recipient) and private key (identity).
func GenerateKeyPair() (recipient string, identity string, err error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("generating age key pair: %w", err)
	}
	return id.Recipient().String(), id.String(), nil
}

// Encrypt encrypts data using the given age recipient (public key).
func Encrypt(reader io.Reader, recipientStr string) (io.Reader, error) {
	recipient, err := age.ParseX25519Recipient(recipientStr)
	if err != nil {
		return nil, fmt.Errorf("parsing recipient: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("creating encryptor: %w", err)
	}
	if _, err := io.Copy(w, reader); err != nil {
		return nil, fmt.Errorf("encrypting: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finalizing encryption: %w", err)
	}
	return &buf, nil
}

// Decrypt decrypts data using the given age identity (private key).
func Decrypt(reader io.Reader, identityStr string) (io.Reader, error) {
	identity, err := age.ParseX25519Identity(identityStr)
	if err != nil {
		return nil, fmt.Errorf("parsing identity: %w", err)
	}

	r, err := age.Decrypt(reader, identity)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}
	return &buf, nil
}

// EncryptWithPassphrase encrypts data using a passphrase (scrypt-based).
func EncryptWithPassphrase(reader io.Reader, passphrase string) (io.Reader, error) {
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt recipient: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("creating encryptor: %w", err)
	}
	if _, err := io.Copy(w, reader); err != nil {
		return nil, fmt.Errorf("encrypting: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finalizing encryption: %w", err)
	}
	return &buf, nil
}

// DecryptWithPassphrase decrypts data encrypted with a passphrase.
func DecryptWithPassphrase(reader io.Reader, passphrase string) (io.Reader, error) {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("creating scrypt identity: %w", err)
	}

	r, err := age.Decrypt(reader, identity)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}
	return &buf, nil
}

// IdentityFromString parses an age identity string.
func IdentityFromString(s string) (age.Identity, error) {
	ids, err := age.ParseIdentities(strings.NewReader(s))
	if err != nil {
		return nil, fmt.Errorf("parsing identity: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no identities found")
	}
	return ids[0], nil
}
