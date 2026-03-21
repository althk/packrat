package crypto

import (
	"os"
	"testing"
)

func TestKeyringRoundTrip(t *testing.T) {
	// Skip if no keyring is available (e.g., headless CI)
	if os.Getenv("DISPLAY") == "" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("skipping keyring test: no display/dbus available")
	}

	identity := "AGE-SECRET-KEY-1TEST"
	if err := StoreKey(identity); err != nil {
		t.Skipf("keyring not available: %v", err)
	}

	got, err := RetrieveKey()
	if err != nil {
		t.Fatalf("RetrieveKey: %v", err)
	}
	if got != identity {
		t.Errorf("retrieved = %q, want %q", got, identity)
	}

	if err := DeleteKey(); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
}
