package keychain

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"
)

const loginAccountPrefix = "login:"

// Login is the secret part of a saved credential.
type Login struct {
	Username string `json:"username"`
	Domain   string `json:"domain,omitempty"`
	Password string `json:"password"`
}

// GetLogin returns a saved login. The boolean is false when none is stored.
func GetLogin(id string) (Login, bool, error) {
	raw, err := keyring.Get(serviceName, loginAccountPrefix+id)
	if err == keyring.ErrNotFound {
		return Login{}, false, nil
	}
	if err != nil {
		return Login{}, false, fmt.Errorf("reading login %s: %w", id, err)
	}
	var login Login
	if err := json.Unmarshal([]byte(raw), &login); err != nil {
		// The error never includes the stored value.
		return Login{}, false, fmt.Errorf("decoding login %s", id)
	}
	return login, true, nil
}

// SetLogin stores a login.
func SetLogin(id string, login Login) error {
	raw, err := json.Marshal(login)
	if err != nil {
		return fmt.Errorf("encoding login %s", id)
	}
	if err := keyring.Set(serviceName, loginAccountPrefix+id, string(raw)); err != nil {
		return fmt.Errorf("storing login %s: %w", id, err)
	}
	return nil
}

// DeleteLogin removes a login, if stored.
func DeleteLogin(id string) error {
	err := keyring.Delete(serviceName, loginAccountPrefix+id)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("deleting login %s: %w", id, err)
	}
	return nil
}
