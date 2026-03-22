package crypto

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	recipient, identity, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if !strings.HasPrefix(recipient, "age1") {
		t.Errorf("recipient should start with age1, got %q", recipient)
	}
	if !strings.HasPrefix(identity, "AGE-SECRET-KEY-1") {
		t.Errorf("identity should start with AGE-SECRET-KEY-1, got %q", identity)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	recipient, identity, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	plaintext := "hello, this is a secret message for packrat!"

	encrypted, err := Encrypt(strings.NewReader(plaintext), recipient)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Encrypted data should be different from plaintext
	encData, _ := io.ReadAll(encrypted)
	if string(encData) == plaintext {
		t.Error("encrypted data should differ from plaintext")
	}

	decrypted, err := Decrypt(bytes.NewReader(encData), identity)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	decData, _ := io.ReadAll(decrypted)
	if string(decData) != plaintext {
		t.Errorf("decrypted = %q, want %q", string(decData), plaintext)
	}
}

func TestEncryptDecryptPassphrase(t *testing.T) {
	passphrase := "correct-horse-battery-staple"
	plaintext := "passphrase-protected secret"

	encrypted, err := EncryptWithPassphrase(strings.NewReader(plaintext), passphrase)
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}

	encData, _ := io.ReadAll(encrypted)

	decrypted, err := DecryptWithPassphrase(bytes.NewReader(encData), passphrase)
	if err != nil {
		t.Fatalf("DecryptWithPassphrase: %v", err)
	}

	decData, _ := io.ReadAll(decrypted)
	if string(decData) != plaintext {
		t.Errorf("decrypted = %q, want %q", string(decData), plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	recipient, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, wrongIdentity, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	encrypted, err := Encrypt(strings.NewReader("secret"), recipient)
	if err != nil {
		t.Fatal(err)
	}

	encData, _ := io.ReadAll(encrypted)
	_, err = Decrypt(bytes.NewReader(encData), wrongIdentity)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestKeyFileSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.key"

	_, identity, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	if err := SaveKeyToFile(path, identity); err != nil {
		t.Fatalf("SaveKeyToFile: %v", err)
	}

	loaded, err := LoadKeyFromFile(path)
	if err != nil {
		t.Fatalf("LoadKeyFromFile: %v", err)
	}

	if loaded != identity {
		t.Errorf("loaded key = %q, want %q", loaded, identity)
	}
}

func TestRecipientFromIdentity(t *testing.T) {
	recipient, identity, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	derived, err := RecipientFromIdentity(identity)
	if err != nil {
		t.Fatalf("RecipientFromIdentity: %v", err)
	}

	if derived != recipient {
		t.Errorf("derived recipient = %q, want %q", derived, recipient)
	}

	_, err = RecipientFromIdentity("not-a-valid-key")
	if err == nil {
		t.Error("expected error for invalid identity")
	}
}
