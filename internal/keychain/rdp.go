package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const rdpAccountPrefix = "rdp:"

// DeleteRDPCredentials removes a per-target login saved by version 1.2.0-beta.2
// or older, if any. Newer versions keep logins in the credentials store.
func DeleteRDPCredentials(hostID string) error {
	err := keyring.Delete(serviceName, rdpAccountPrefix+hostID)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("deleting RDP login for %s: %w", hostID, err)
	}
	return nil
}
