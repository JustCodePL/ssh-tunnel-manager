package dns

import "context"

// SystemServiceStatus is safe to expose through Wails. The service is
// machine-wide, while this snapshot is read by whichever user is running the
// GUI.
type SystemServiceStatus struct {
	Available        bool   `json:"available"`
	Installed        bool   `json:"installed"`
	ApprovalRequired bool   `json:"approvalRequired"`
	Current          bool   `json:"current"`
	State            string `json:"state"`
	Message          string `json:"message"`
}

func GetSystemServiceStatus() SystemServiceStatus {
	return getSystemServiceStatus()
}

func InstallSystemService(ctx context.Context) error {
	return installSystemService(ctx)
}

func UninstallSystemService(ctx context.Context) error {
	return uninstallSystemService(ctx)
}

func OpenSystemServiceSettings() error {
	return openSystemServiceSettings()
}
