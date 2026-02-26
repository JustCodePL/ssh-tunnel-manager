package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const serviceName = "ssh-tunnel-manager"

// GetPassphrase retrieves a stored passphrase for the given key path.
// Returns empty string and no error if not found.
func GetPassphrase(keyPath string) (string, error) {
	pass, err := keyring.Get(serviceName, keyPath)
	if err == keyring.ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading keychain for %s: %w", keyPath, err)
	}
	return pass, nil
}

// SetPassphrase stores a passphrase for the given key path in the OS keychain.
func SetPassphrase(keyPath, passphrase string) error {
	if err := keyring.Set(serviceName, keyPath, passphrase); err != nil {
		return fmt.Errorf("storing passphrase for %s: %w", keyPath, err)
	}
	return nil
}

// DeletePassphrase removes a stored passphrase for the given key path.
func DeletePassphrase(keyPath string) error {
	err := keyring.Delete(serviceName, keyPath)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
