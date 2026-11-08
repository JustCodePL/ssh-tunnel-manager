package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/keychain"
	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/ssh"
)

const protocolVNC = "vnc"

// CredentialView is a saved login as the frontend sees it. The password never
// leaves the backend here.
type CredentialView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Username    string   `json:"username"`
	Domain      string   `json:"domain"`
	HasPassword bool     `json:"hasPassword"`
	Protocols   []string `json:"protocols"`
	UsedBy      int      `json:"usedBy"`
}

// CredentialInput creates or edits a login. An empty ID creates one, and an
// empty Password keeps the stored password.
type CredentialInput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Username  string   `json:"username"`
	Domain    string   `json:"domain"`
	Password  string   `json:"password"`
	Protocols []string `json:"protocols"`
}

func validateCredentialInput(input CredentialInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("a login needs a name")
	}
	if len(input.Protocols) == 0 {
		return errors.New("choose at least one protocol")
	}
	for _, protocol := range input.Protocols {
		if protocol != config.ForwardProtocolRDP && protocol != config.ForwardProtocolSSH && protocol != protocolVNC {
			return fmt.Errorf("unknown protocol %q", protocol)
		}
	}
	return nil
}

// ListCredentials returns all saved logins.
func (a *App) ListCredentials() ([]CredentialView, error) {
	p := a.prefs.Get()
	views := make([]CredentialView, 0, len(p.Credentials))
	for _, credential := range p.Credentials {
		view := CredentialView{ID: credential.ID, Name: credential.Name, Protocols: credential.Protocols}
		login, found, err := keychain.GetLogin(credential.ID)
		if err != nil {
			return nil, err
		}
		if found {
			view.Username, view.Domain, view.HasPassword = login.Username, login.Domain, login.Password != ""
		}
		for _, id := range p.CredentialBindings {
			if id == credential.ID {
				view.UsedBy++
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// SaveCredential creates or updates a login.
func (a *App) SaveCredential(input CredentialInput) (CredentialView, error) {
	if err := validateCredentialInput(input); err != nil {
		return CredentialView{}, err
	}
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()

	p := a.prefs.Get()
	index := slices.IndexFunc(p.Credentials, func(c prefs.Credential) bool { return c.ID == input.ID })
	if input.ID != "" && index < 0 {
		return CredentialView{}, fmt.Errorf("no login %s", input.ID)
	}
	login := keychain.Login{Username: input.Username, Domain: input.Domain, Password: input.Password}
	if input.ID == "" {
		if input.Password == "" {
			return CredentialView{}, errors.New("a new login needs a password")
		}
		input.ID = newCredentialID()
	} else if input.Password == "" {
		stored, found, err := keychain.GetLogin(input.ID)
		if err != nil {
			return CredentialView{}, err
		}
		if found {
			login.Password = stored.Password
		}
	}
	if err := keychain.SetLogin(input.ID, login); err != nil {
		return CredentialView{}, err
	}
	credentials := slices.Clone(p.Credentials)
	entry := prefs.Credential{ID: input.ID, Name: strings.TrimSpace(input.Name), Protocols: input.Protocols}
	if index >= 0 {
		credentials[index] = entry
	} else {
		credentials = append(credentials, entry)
	}
	p.Credentials = credentials
	if err := a.prefs.Set(p); err != nil {
		return CredentialView{}, err
	}
	a.emit(eventSettingsChange)
	return CredentialView{
		ID: entry.ID, Name: entry.Name, Username: login.Username, Domain: login.Domain,
		HasPassword: login.Password != "", Protocols: entry.Protocols,
	}, nil
}

// DeleteCredential removes a login and every assignment of it.
func (a *App) DeleteCredential(id string) error {
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	p := a.prefs.Get()
	p.Credentials = slices.DeleteFunc(slices.Clone(p.Credentials), func(c prefs.Credential) bool { return c.ID == id })
	bindings := make(map[string]string, len(p.CredentialBindings))
	for key, value := range p.CredentialBindings {
		if value != id {
			bindings[key] = value
		}
	}
	p.CredentialBindings = bindings
	if err := keychain.DeleteLogin(id); err != nil {
		return err
	}
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emit(eventSettingsChange)
	return nil
}

func newCredentialID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("reading random bytes: %v", err))
	}
	return hex.EncodeToString(raw)
}

func bindingKey(protocol, stateKey string) string {
	return protocol + ":" + stateKey
}

// bindingTarget returns the key a protocol's login is assigned under. An SSH
// login belongs to the tunnel, an RDP login to the tunnel or one forward.
func (a *App) bindingTarget(protocol, tunnelID string, target ForwardTarget) (string, error) {
	switch protocol {
	case config.ForwardProtocolSSH:
		if _, ok := a.store.GetTunnel(tunnelID); !ok {
			return "", &tunnelNotFoundError{tunnelID}
		}
		return bindingKey(protocol, tunnelID), nil
	case config.ForwardProtocolRDP:
		key, _, err := a.resolveRDPTarget(tunnelID, target)
		if err != nil {
			return "", err
		}
		return bindingKey(protocol, key), nil
	}
	return "", fmt.Errorf("binding is not available for %q yet", protocol)
}

// BindCredential assigns a login to a target for one protocol, and marks the
// login as used for it. An empty ID removes the assignment: an RDP target then
// asks at connect time, and a tunnel uses only its key or agent.
func (a *App) BindCredential(protocol, tunnelID string, target ForwardTarget, credentialID string) error {
	key, err := a.bindingTarget(protocol, tunnelID, target)
	if err != nil {
		return err
	}
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	if err := a.setBinding(key, credentialID); err != nil {
		return err
	}
	if credentialID != "" {
		if err := a.addCredentialProtocol(credentialID, protocol); err != nil {
			return err
		}
	}
	a.emit(eventSettingsChange)
	return nil
}

// addCredentialProtocol marks a login as used for a protocol. The caller holds
// credentialsMu.
func (a *App) addCredentialProtocol(id, protocol string) error {
	p := a.prefs.Get()
	index := slices.IndexFunc(p.Credentials, func(c prefs.Credential) bool { return c.ID == id })
	if index < 0 || slices.Contains(p.Credentials[index].Protocols, protocol) {
		return nil
	}
	credentials := slices.Clone(p.Credentials)
	credentials[index].Protocols = append(slices.Clone(credentials[index].Protocols), protocol)
	p.Credentials = credentials
	return a.prefs.Set(p)
}

// setBinding stores one assignment. The caller holds credentialsMu.
func (a *App) setBinding(key, credentialID string) error {
	p := a.prefs.Get()
	if credentialID != "" && !slices.ContainsFunc(p.Credentials, func(c prefs.Credential) bool { return c.ID == credentialID }) {
		return fmt.Errorf("no login %s", credentialID)
	}
	bindings := make(map[string]string, len(p.CredentialBindings)+1)
	for k, v := range p.CredentialBindings {
		bindings[k] = v
	}
	if credentialID == "" {
		delete(bindings, key)
	} else {
		bindings[key] = credentialID
	}
	p.CredentialBindings = bindings
	return a.prefs.Set(p)
}

// GetCredentialBinding returns the login assigned to a target, or an empty ID.
func (a *App) GetCredentialBinding(protocol, tunnelID string, target ForwardTarget) (string, error) {
	key, err := a.bindingTarget(protocol, tunnelID, target)
	if err != nil {
		return "", err
	}
	return a.prefs.Get().CredentialBindings[key], nil
}

// sshLogin returns the password login assigned to a tunnel, or nil when it has
// none. A login whose secret is gone from the keychain counts as none, so the
// tunnel still tries its key and agent.
func (a *App) sshLogin(tunnelID string) (*ssh.PasswordLogin, error) {
	id := a.prefs.Get().CredentialBindings[bindingKey(config.ForwardProtocolSSH, tunnelID)]
	if id == "" {
		return nil, nil
	}
	login, found, err := keychain.GetLogin(id)
	if err != nil {
		return nil, err
	}
	if !found || login.Password == "" {
		slog.Warn("assigned login has no password in the keychain", "tunnel", tunnelID)
		return nil, nil
	}
	return &ssh.PasswordLogin{
		Username: login.Username,
		Password: login.Password,
	}, nil
}

// SSHHostKey is a host key the user accepted for a tunnel.
type SSHHostKey struct {
	Address     string `json:"address"`
	Fingerprint string `json:"fingerprint"`
}

// GetSSHHostKeys returns the pinned host keys of a tunnel.
func (a *App) GetSSHHostKeys(tunnelID string) []SSHHostKey {
	pinned := a.prefs.Get().SSHHostKeys[tunnelID]
	keys := make([]SSHHostKey, 0, len(pinned))
	for address, fingerprint := range pinned {
		keys = append(keys, SSHHostKey{Address: address, Fingerprint: fingerprint})
	}
	slices.SortFunc(keys, func(x, y SSHHostKey) int { return strings.Compare(x.Address, y.Address) })
	return keys
}

// ForgetSSHHostKeys removes a tunnel's pinned host keys, so the next connection
// asks about any server that is not in known_hosts.
func (a *App) ForgetSSHHostKeys(tunnelID string) error {
	a.credentialsMu.Lock()
	defer a.credentialsMu.Unlock()
	p := a.prefs.Get()
	if _, ok := p.SSHHostKeys[tunnelID]; !ok {
		return nil
	}
	p.SSHHostKeys = withSSHHostKeys(p.SSHHostKeys, tunnelID, nil)
	if err := a.prefs.Set(p); err != nil {
		return err
	}
	a.emit(eventSettingsChange)
	return nil
}

// withSSHHostKeys returns a copy of the pins with one tunnel's keys replaced, or
// removed when nil.
func withSSHHostKeys(pins map[string]map[string]string, tunnelID string, keys map[string]string) map[string]map[string]string {
	updated := make(map[string]map[string]string, len(pins)+1)
	for id, value := range pins {
		updated[id] = value
	}
	if keys == nil {
		delete(updated, tunnelID)
	} else {
		updated[tunnelID] = keys
	}
	return updated
}

// boundRDPLogin returns the login assigned to an RDP target.
func (a *App) boundRDPLogin(stateKey string) (string, keychain.Login, bool, error) {
	id := a.prefs.Get().CredentialBindings[bindingKey(config.ForwardProtocolRDP, stateKey)]
	if id == "" {
		return "", keychain.Login{}, false, nil
	}
	login, found, err := keychain.GetLogin(id)
	if err != nil || !found {
		return id, keychain.Login{}, false, err
	}
	return id, login, true, nil
}

// credentialName returns the display name of a login.
func (a *App) credentialName(id string) string {
	for _, c := range a.prefs.Get().Credentials {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// moveCredentialBindings follows a rename of a target's key, for its RDP login
// and, when the key is the tunnel's own, its SSH login.
func moveCredentialBindings(bindings map[string]string, moves map[string]string) map[string]string {
	updated := make(map[string]string, len(bindings))
	for key, value := range bindings {
		updated[key] = value
	}
	for from, to := range moves {
		for _, protocol := range []string{config.ForwardProtocolRDP, config.ForwardProtocolSSH} {
			fromKey, toKey := bindingKey(protocol, from), bindingKey(protocol, to)
			if value, ok := updated[fromKey]; ok {
				delete(updated, fromKey)
				updated[toKey] = value
			}
		}
	}
	return updated
}

func removeCredentialBindings(bindings map[string]string, stateKeys []string) (map[string]string, bool) {
	updated := make(map[string]string, len(bindings))
	for key, value := range bindings {
		updated[key] = value
	}
	changed := false
	for _, stateKey := range stateKeys {
		for _, protocol := range []string{config.ForwardProtocolRDP, config.ForwardProtocolSSH} {
			key := bindingKey(protocol, stateKey)
			if _, ok := updated[key]; ok {
				delete(updated, key)
				changed = true
			}
		}
	}
	return updated, changed
}

func logBindingFailure(target string, err error) {
	slog.Warn("saving login assignment failed", "target", target, "error", err)
}
