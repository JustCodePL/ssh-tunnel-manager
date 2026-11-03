package keychain

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"
)

const rdpAccountPrefix = "rdp:"

// RDPCredentials is the login for a host's remote desktop.
type RDPCredentials struct {
	Username string `json:"username"`
	Domain   string `json:"domain,omitempty"`
	Password string `json:"password"`
}

// GetRDPCredentials returns the stored login for a host. The boolean is false
// when none is stored.
func GetRDPCredentials(hostID string) (RDPCredentials, bool, error) {
	raw, err := keyring.Get(serviceName, rdpAccountPrefix+hostID)
	if err == keyring.ErrNotFound {
		return RDPCredentials{}, false, nil
	}
	if err != nil {
		return RDPCredentials{}, false, fmt.Errorf("reading RDP login for %s: %w", hostID, err)
	}
	var credentials RDPCredentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		// The error never includes the stored value.
		return RDPCredentials{}, false, fmt.Errorf("decoding RDP login for %s", hostID)
	}
	return credentials, true, nil
}

// SetRDPCredentials stores the login for a host.
func SetRDPCredentials(hostID string, credentials RDPCredentials) error {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("encoding RDP login for %s", hostID)
	}
	if err := keyring.Set(serviceName, rdpAccountPrefix+hostID, string(raw)); err != nil {
		return fmt.Errorf("storing RDP login for %s: %w", hostID, err)
	}
	return nil
}

// DeleteRDPCredentials removes the stored login for a host, if any.
func DeleteRDPCredentials(hostID string) error {
	err := keyring.Delete(serviceName, rdpAccountPrefix+hostID)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("deleting RDP login for %s: %w", hostID, err)
	}
	return nil
}
